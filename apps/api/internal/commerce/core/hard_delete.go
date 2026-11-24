package core

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"gorm.io/gorm"
)

type HardDeleteResult struct {
	Counts    map[string]int64 `json:"counts"`
	TotalRows int64            `json:"total_rows"`
	DryRun    bool             `json:"dry_run"`
}

func (s *Service) ConfigureOrganizationHardDelete(enabled bool, appEnv string) {
	s.organizationHardDeleteEnabled, s.appEnv = enabled, appEnv
}

// NewOrganizationHardDeleteOperatorService is for trusted processes with direct
// database credentials, never HTTP handlers. It changes only the actor authority;
// the environment guards, transaction, selection and deletion are identical.
func NewOrganizationHardDeleteOperatorService(db *gorm.DB, enabled bool, appEnv string) *Service {
	s := NewService(db, nil)
	s.ConfigureOrganizationHardDelete(enabled, appEnv)
	s.organizationHardDeleteOperator = true
	return s
}

func (s *Service) authorizeOrganizationHardDelete(db *gorm.DB, actor auth.CurrentUser, orgID uuid.UUID) error {
	if s.organizationHardDeleteOperator {
		return nil
	}
	return hardDeleteAuthorize(db, actor, orgID)
}

func (s *Service) hardDeleteGuard() error {
	if strings.EqualFold(strings.TrimSpace(s.appEnv), "production") {
		return httperror.Forbidden("Organization hard delete is unavailable in production")
	}
	if !s.organizationHardDeleteEnabled {
		return httperror.Forbidden("Organization hard delete is disabled")
	}
	return nil
}

func hardDeleteAuthorize(db *gorm.DB, actor auth.CurrentUser, orgID uuid.UUID) error {
	// Re-read global privilege; a tenant role or a stale token cannot grant it.
	var admin int64
	if err := db.Table("users").Where("id = ? AND role = ? AND status = ?", actor.ID, authz.PlatformAdmin, "active").Count(&admin).Error; err != nil {
		return err
	}
	if admin > 0 {
		return nil
	}
	var owners int64
	if err := db.Table("organization_memberships").Where("organization_id = ? AND user_id = ? AND is_owner = ? AND status = ?", orgID, actor.ID, true, "active").Count(&owners).Error; err != nil {
		return err
	}
	if owners == 0 {
		return httperror.Forbidden("Only an organization owner or platform administrator can delete this organization")
	}
	return nil
}

func (s *Service) OrganizationHardDeleteCapability(ctx context.Context, actor auth.CurrentUser, orgID uuid.UUID) bool {
	return s.hardDeleteGuard() == nil && hardDeleteAuthorize(s.db.WithContext(ctx), actor, orgID) == nil
}

type deleteTable struct {
	Name string
	Org  bool
	Keys []string
	Temp string
}
type deleteFK struct {
	Child, Parent, ChildColumn, ParentColumn, Action string
	OID                                              int64 `gorm:"column:oid"`
	Position                                         int
}

func qi(s string) string       { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }
func tableSQL(s string) string { return `"public".` + qi(s) }
func (t *deleteTable) key(alias string) string {
	parts := make([]string, len(t.Keys))
	for i, k := range t.Keys {
		parts[i] = alias + "." + qi(k)
	}
	return "jsonb_build_array(" + strings.Join(parts, ",") + ")"
}
func (t *deleteTable) selected(alias string) string {
	return "EXISTS (SELECT 1 FROM " + qi(t.Temp) + " selected WHERE selected.k = " + t.key(alias) + ")"
}

// Read the applied migration schema, including ALTER TABLE and composite FKs.
// No application model list is used: new tenant tables are included automatically.
func readDeleteSchema(tx *gorm.DB) (map[string]*deleteTable, []deleteFK, error) {
	var columns []struct {
		TableName, ColumnName string
		IsKey                 bool
	}
	err := tx.Raw(`SELECT c.relname AS table_name, a.attname AS column_name,
 EXISTS(SELECT 1 FROM pg_index i WHERE i.indrelid=c.oid AND i.indisprimary AND a.attnum=ANY(i.indkey)) AS is_key
 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace JOIN pg_attribute a ON a.attrelid=c.oid
 WHERE n.nspname='public' AND c.relkind='r' AND a.attnum>0 AND NOT a.attisdropped ORDER BY c.relname,a.attnum`).Scan(&columns).Error
	if err != nil {
		return nil, nil, err
	}
	tables := map[string]*deleteTable{}
	for _, c := range columns {
		t := tables[c.TableName]
		if t == nil {
			t = &deleteTable{Name: c.TableName}
			tables[c.TableName] = t
		}
		if c.ColumnName == "organization_id" {
			t.Org = true
		}
		if c.IsKey {
			t.Keys = append(t.Keys, c.ColumnName)
		}
	}
	var fks []deleteFK
	err = tx.Raw(`SELECT f.oid::bigint AS oid, ch.relname AS child, pa.relname AS parent, ca.attname AS child_column, ra.attname AS parent_column, f.confdeltype::text AS action, k.ord AS position
 FROM pg_constraint f JOIN pg_class ch ON ch.oid=f.conrelid JOIN pg_namespace cn ON cn.oid=ch.relnamespace
 JOIN pg_class pa ON pa.oid=f.confrelid JOIN pg_namespace pn ON pn.oid=pa.relnamespace
 CROSS JOIN LATERAL unnest(f.conkey,f.confkey) WITH ORDINALITY AS k(childnum,parentnum,ord)
 JOIN pg_attribute ca ON ca.attrelid=ch.oid AND ca.attnum=k.childnum JOIN pg_attribute ra ON ra.attrelid=pa.oid AND ra.attnum=k.parentnum
 WHERE f.contype='f' AND cn.nspname='public' AND pn.nspname='public' ORDER BY f.oid,k.ord`).Scan(&fks).Error
	return tables, fks, err
}

// HardDeleteOrganization snapshots the exact rows before deletion in one locked
// transaction. dryRun uses the identical plan and boundary checks, but no DELETE.
func (s *Service) HardDeleteOrganization(ctx context.Context, actor auth.CurrentUser, orgID uuid.UUID, confirmSlug string, dryRun ...bool) (HardDeleteResult, error) {
	result := HardDeleteResult{Counts: map[string]int64{}, DryRun: len(dryRun) > 0 && dryRun[0]}
	if err := s.hardDeleteGuard(); err != nil {
		return result, err
	}
	var slug string
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorizeOrganizationHardDelete(tx, actor, orgID); err != nil {
			return err
		}
		if err := tx.Table("organizations").Select("slug").Where("id = ?", orgID).Take(&slug).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return httperror.NotFound("Organization not found")
			}
			return err
		}
		if confirmSlug == "" || confirmSlug != slug {
			return httperror.BadRequest("confirm_slug must exactly match the organization slug")
		}
		if tx.Dialector.Name() != "postgres" {
			return httperror.Conflict("Organization hard delete requires PostgreSQL")
		}
		// Block writers and schema changes while discovering, counting and deleting.
		// Lock all public tables in deterministic order to also protect indirect rows.
		var names []string
		if err := tx.Raw(`SELECT tablename FROM pg_tables WHERE schemaname='public' ORDER BY tablename`).Scan(&names).Error; err != nil {
			return err
		}
		lockNames := make([]string, len(names))
		for i, n := range names {
			lockNames[i] = tableSQL(n)
		}
		if err := tx.Exec("SET LOCAL lock_timeout = '5s'").Error; err != nil {
			return err
		}
		if err := tx.Exec("LOCK TABLE " + strings.Join(lockNames, ",") + " IN SHARE ROW EXCLUSIVE MODE").Error; err != nil {
			return err
		}
		// Recheck after acquiring locks to close authorization/slug races.
		if err := s.authorizeOrganizationHardDelete(tx, actor, orgID); err != nil {
			return err
		}
		var current string
		if err := tx.Table("organizations").Select("slug").Where("id = ?", orgID).Take(&current).Error; err != nil {
			return err
		}
		if current != confirmSlug {
			return httperror.BadRequest("confirm_slug must exactly match the organization slug")
		}
		tables, fks, err := readDeleteSchema(tx)
		if err != nil {
			return err
		}
		// Group composite foreign keys into one equality predicate.
		type edge struct{ child, parent, action, join string }
		var edges []edge
		for i := 0; i < len(fks); {
			f := fks[i]
			var joins []string
			j := i
			for j < len(fks) && fks[j].OID == f.OID {
				joins = append(joins, "c."+qi(fks[j].ChildColumn)+" = p."+qi(fks[j].ParentColumn))
				j++
			}
			edges = append(edges, edge{f.Child, f.Parent, f.Action, strings.Join(joins, " AND ")})
			i = j
		}
		selected := map[string]bool{"organizations": true, "users": true}
		for n, t := range tables {
			if t.Org {
				selected[n] = true
			}
		}
		for changed := true; changed; {
			changed = false
			for _, e := range edges {
				if selected[e.parent] && e.child != "users" && !selected[e.child] {
					selected[e.child] = true
					changed = true
				}
			}
		}
		var plan []string
		for n := range selected {
			plan = append(plan, n)
		}
		sort.Strings(plan)
		// Seed the live FK traversal with the migration-derived audited order.
		ranks := map[string]int{}
		for i, name := range migrationOrganizationDeleteOrder {
			ranks[name] = i + 1
		}
		sort.SliceStable(plan, func(i, j int) bool {
			a, b := ranks[plan[i]], ranks[plan[j]]
			if a == 0 {
				return false
			}
			if b == 0 {
				return true
			}
			return a < b
		})
		for i, n := range plan {
			t := tables[n]
			if t == nil || len(t.Keys) == 0 {
				return httperror.Conflict("Deletion requires a primary key on every affected table")
			}
			t.Temp = fmt.Sprintf("org_delete_%d", i)
			if err := tx.Exec("CREATE TEMP TABLE " + qi(t.Temp) + " (k jsonb PRIMARY KEY) ON COMMIT DROP").Error; err != nil {
				return err
			}
			predicate := "false"
			args := []interface{}{}
			switch n {
			case "organizations":
				predicate = "c.id = ?"
				args = append(args, orgID)
			case "users":
				predicate = `c.role <> 'platform_admin' AND (c.organization_id = ? OR EXISTS (SELECT 1 FROM organization_memberships m WHERE m.user_id=c.id AND m.organization_id=?)) AND NOT EXISTS (SELECT 1 FROM organization_memberships m WHERE m.user_id=c.id AND m.organization_id<>?)`
				args = append(args, orgID, orgID, orgID)
			default:
				if t.Org {
					predicate = "c.organization_id = ?"
					args = append(args, orgID)
				}
			}
			if err := tx.Exec("INSERT INTO "+qi(t.Temp)+" SELECT "+t.key("c")+" FROM "+tableSQL(n)+" c WHERE "+predicate, args...).Error; err != nil {
				return err
			}
		}
		// Closure follows only tables without organization_id. Scoped tables are
		// always selected by their own tenant, never by another tenant's reference.
		for changed := true; changed; {
			changed = false
			for _, e := range edges {
				c, p := tables[e.child], tables[e.parent]
				if !selected[e.child] || !selected[e.parent] || c.Org || e.child == "users" || e.child == "organizations" {
					continue
				}
				r := tx.Exec("INSERT INTO " + qi(c.Temp) + " SELECT DISTINCT " + c.key("c") + " FROM " + tableSQL(c.Name) + " c JOIN " + tableSQL(p.Name) + " p ON " + e.join + " WHERE " + p.selected("p") + " ON CONFLICT DO NOTHING")
				if r.Error != nil {
					return r.Error
				}
				if r.RowsAffected > 0 {
					changed = true
				}
			}
		}
		// Retained shared/global users lose their legacy org pointer; memberships in
		// other organizations and platform administrator accounts remain intact.
		// No other surviving record may be changed by CASCADE or SET NULL.
		for _, e := range edges {
			if !selected[e.parent] {
				continue
			}
			if e.child == "users" && e.parent == "organizations" {
				continue
			}
			c, p := tables[e.child], tables[e.parent]
			childSelected := "false"
			if selected[e.child] {
				childSelected = c.selected("c")
			}
			var count int64
			if err := tx.Raw("SELECT count(*) FROM " + tableSQL(c.Name) + " c JOIN " + tableSQL(p.Name) + " p ON " + e.join + " WHERE " + p.selected("p") + " AND NOT (" + childSelected + ")").Scan(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return httperror.Conflict("Deletion would affect surviving records in " + c.Name)
			}
		}
		// Topological child-before-parent order. SET NULL edges do not constrain
		// deletion order; snapshots use primary keys so those updates cannot lose rows.
		state := map[string]int{}
		var order []string
		var visit func(string) error
		visit = func(n string) error {
			if state[n] == 2 {
				return nil
			}
			if state[n] == 1 {
				return httperror.Conflict("Unsupported foreign-key cycle; deletion refused")
			}
			state[n] = 1
			for _, e := range edges {
				if e.parent == n && e.child != n && selected[e.child] && e.action != "n" {
					if err := visit(e.child); err != nil {
						return err
					}
				}
			}
			state[n] = 2
			order = append(order, n)
			return nil
		}
		for _, n := range plan {
			if err := visit(n); err != nil {
				return err
			}
		}
		// Organization removal is always the final statement, including future
		// tenant-column tables that do not themselves have an organization FK.
		for i, name := range order {
			if name == "organizations" {
				order = append(append(order[:i:i], order[i+1:]...), name)
				break
			}
		}
		for _, n := range plan {
			var count int64
			if err := tx.Table(qi(tables[n].Temp)).Count(&count).Error; err != nil {
				return err
			}
			result.Counts[n] = count
			result.TotalRows += count
		}
		if result.DryRun {
			return nil
		}
		if err := tx.Exec("UPDATE public.users SET organization_id=NULL WHERE organization_id=? AND NOT ("+tables["users"].selected("users")+")", orgID).Error; err != nil {
			return err
		}
		for _, n := range order {
			t := tables[n]
			if err := tx.Exec("DELETE FROM " + tableSQL(n) + " c WHERE " + t.selected("c")).Error; err != nil {
				return err
			}
		}
		for _, n := range plan {
			t := tables[n]
			if t.Org {
				var count int64
				if err := tx.Table(tableSQL(n)).Where("organization_id = ?", orgID).Count(&count).Error; err != nil {
					return err
				}
				if count != 0 {
					return httperror.Conflict("Organization rows remain in " + n)
				}
			}
		}
		return nil
	})
	if err == nil && !result.DryRun {
		logger := s.log
		if logger == nil {
			logger = slog.Default()
		}
		actorKind := "user"
		if s.organizationHardDeleteOperator {
			actorKind = "database_operator"
		}
		logger.InfoContext(ctx, "organization hard deleted", "actor_kind", actorKind, "actor_id", actor.ID, "organization_id", orgID, "slug", slug, "total_rows", result.TotalRows)
	}
	return result, err
}
