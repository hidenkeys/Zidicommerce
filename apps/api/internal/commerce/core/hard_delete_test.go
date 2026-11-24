package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/migrations"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func requireDeleteStatus(t *testing.T, err error, status int) {
	t.Helper()
	var apiErr httperror.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != status {
		t.Fatalf("wanted HTTP %d, got %v", status, err)
	}
}

func TestOrganizationHardDeleteGuards(t *testing.T) {
	for _, tc := range []struct {
		name    string
		enabled bool
		env     string
	}{
		{"disabled", false, "staging"}, {"production", true, "production"}, {"production normalized", true, " Production "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewService(nil, SafeTestProvider{})
			s.ConfigureOrganizationHardDelete(tc.enabled, tc.env)
			_, err := s.HardDeleteOrganization(context.Background(), auth.CurrentUser{}, uuid.New(), "test")
			var apiErr httperror.APIError
			if !errors.As(err, &apiErr) || (apiErr.StatusCode != 403 && apiErr.StatusCode != 409) {
				t.Fatalf("unsafe configuration should refuse before DB access, got %v", err)
			}
		})
	}
}

// Like internal/migrations/postgres_test.go, this exercises the repository's
// real migrations only with an explicitly supplied, dedicated empty database.
// It never uses DATABASE_URL or a developer's normal database implicitly.
func TestOrganizationHardDeletePostgres(t *testing.T) {
	dsn := os.Getenv("ZIDI_HARD_DELETE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set ZIDI_HARD_DELETE_TEST_DATABASE_URL to a dedicated empty PostgreSQL database")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	var existing string
	if err := db.Raw(`SELECT COALESCE(to_regclass('public.schema_migrations')::text,'')`).Scan(&existing).Error; err != nil {
		t.Fatal(err)
	}
	if existing != "" {
		t.Fatal("hard-delete test database must be empty and dedicated to this test")
	}
	ctx := context.Background()
	if err := migrations.Run(ctx, sqlDB, filepath.Join("..", "..", "..", "..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if err := db.Exec(sql, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	org, other := uuid.New(), uuid.New()
	owner, shared, admin, member := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	slug := "hard-delete-" + uuid.NewString()
	exec(`INSERT INTO organizations(id,name,slug) VALUES (?, 'Delete fixture', ?), (?, 'Keep fixture', ?)`, org, slug, other, slug+"-keep")
	for _, u := range []struct {
		id   uuid.UUID
		role string
	}{{owner, "merchant_admin"}, {shared, "viewer"}, {admin, "platform_admin"}, {member, "viewer"}} {
		exec(`INSERT INTO users(id,organization_id,email,password_hash,role) VALUES (?, ?, ?, 'test-only', ?)`, u.id, org, u.id.String()+"@example.invalid", u.role)
	}
	exec(`INSERT INTO organization_memberships(organization_id,user_id,role,is_owner) VALUES (?,?,'merchant_admin',true),(?,?,'viewer',false),(?,?,'viewer',false),(?,?,'viewer',false)`, org, owner, org, shared, other, shared, org, member)
	exec(`INSERT INTO organization_invitations(organization_id,email,role,token_hash,invited_by_user_id,expires_at) VALUES (?, 'pending@example.invalid','viewer', ?, ?, now()+interval '1 day')`, org, uuid.NewString(), owner)
	exec(`INSERT INTO audit_logs(organization_id,actor_user_id,target_type,action) VALUES (?,?,'organization','fixture')`, org, owner)
	exec(`INSERT INTO background_jobs(organization_id,job_type,status,locked_by,locked_at) VALUES (?, 'test','processing','fixture-worker',now()), (?, 'keep','queued','',NULL)`, org, other)
	exec(`INSERT INTO payment_provider_secrets(organization_id,provider,secret_name,ciphertext,nonce) VALUES (?, 'test','key','encrypted-fixture','nonce-fixture')`, org)
	source, botID, version := uuid.New(), uuid.New(), uuid.New()
	exec(`INSERT INTO merchant_document_sources(id,organization_id,title,source_type,status) VALUES (?,?,'Fixture','manual','draft')`, source, org)
	exec(`INSERT INTO merchant_document_chunks(organization_id,document_source_id,chunk_index,content,status) VALUES (?,?,0,'fixture','draft')`, org, source)
	exec(`INSERT INTO bots(id,organization_id,name) VALUES (?,?,'Fixture')`, botID, org)
	exec(`INSERT INTO bot_versions(id,organization_id,bot_id,version_number) VALUES (?,?,?,1)`, version, org, botID)
	// The published version forms a real SET NULL cycle with bot_versions.
	exec(`UPDATE bots SET published_version_id=? WHERE id=?`, version, botID)
	// Prove child-of-child discovery and composite keys without organization_id.
	exec(`CREATE TABLE hard_delete_test_children (parent_id uuid PRIMARY KEY REFERENCES merchant_document_sources(id) ON DELETE RESTRICT, marker text NOT NULL)`)
	exec(`CREATE TABLE hard_delete_test_grandchildren (parent_id uuid NOT NULL REFERENCES hard_delete_test_children(parent_id), sequence integer NOT NULL, PRIMARY KEY(parent_id,sequence))`)
	exec(`INSERT INTO hard_delete_test_children VALUES (?, 'delete')`, source)
	exec(`INSERT INTO hard_delete_test_grandchildren VALUES (?,1)`, source)
	exec(`INSERT INTO password_recovery(user_id,token_digest) VALUES (?,decode('ab','hex'))`, owner)
	s := NewService(db, SafeTestProvider{})
	s.ConfigureOrganizationHardDelete(true, "staging")
	actor := auth.CurrentUser{ID: owner, OrganizationID: org, Role: authz.MerchantAdmin}
	_, err = s.HardDeleteOrganization(ctx, actor, org, "wrong")
	requireDeleteStatus(t, err, 400)
	_, err = s.HardDeleteOrganization(ctx, auth.CurrentUser{ID: member, OrganizationID: org, Role: authz.Viewer}, org, slug)
	requireDeleteStatus(t, err, 403)
	// A merchant admin without the owner bit is still forbidden.
	_, err = s.HardDeleteOrganization(ctx, auth.CurrentUser{ID: member, OrganizationID: org, Role: authz.MerchantAdmin}, org, slug)
	requireDeleteStatus(t, err, 403)
	// A malformed cross-tenant reference must fail closed even when PostgreSQL
	// would happily cascade into the surviving organization's data.
	exec(`CREATE TABLE hard_delete_test_cross_tenant (id uuid PRIMARY KEY, organization_id uuid NOT NULL REFERENCES organizations(id), source_id uuid NOT NULL REFERENCES merchant_document_sources(id) ON DELETE CASCADE)`)
	crossID := uuid.New()
	exec(`INSERT INTO hard_delete_test_cross_tenant VALUES (?,?,?)`, crossID, other, source)
	for _, dryRun := range []bool{true, false} {
		_, err = s.HardDeleteOrganization(ctx, actor, org, slug, dryRun)
		requireDeleteStatus(t, err, 409)
		for _, row := range []struct {
			table string
			id    uuid.UUID
		}{
			{"organizations", org},
			{"merchant_document_sources", source},
			{"hard_delete_test_cross_tenant", crossID},
		} {
			var count int64
			if err := db.Table(row.table).Where("id = ?", row.id).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("refused deletion changed %s row %s", row.table, row.id)
			}
		}
	}
	exec(`DELETE FROM hard_delete_test_cross_tenant WHERE id = ?`, crossID)
	before := map[string]int64{}
	var tables []string
	if err := db.Raw(`SELECT table_name FROM information_schema.columns WHERE table_schema='public' AND column_name='organization_id' ORDER BY table_name`).Scan(&tables).Error; err != nil {
		t.Fatal(err)
	}
	for _, table := range tables {
		var count int64
		if err := db.Table(table).Where("organization_id = ?", org).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		before[table] = count
	}
	dry, err := s.HardDeleteOrganization(ctx, actor, org, slug, true)
	if err != nil {
		t.Fatal(err)
	}
	if !dry.DryRun || dry.TotalRows == 0 || dry.Counts["organizations"] != 1 {
		t.Fatalf("invalid dry-run result: %+v", dry)
	}
	for table, count := range before {
		var got int64
		if err := db.Table(table).Where("organization_id = ?", org).Count(&got).Error; err != nil {
			t.Fatal(err)
		}
		if got != count {
			t.Errorf("dry run changed %s: before %d after %d", table, count, got)
		}
	}
	for _, table := range []string{"hard_delete_test_children", "hard_delete_test_grandchildren", "password_recovery"} {
		if dry.Counts[table] != 1 {
			t.Errorf("dry run should include indirect %s, got %d", table, dry.Counts[table])
		}
	}
	deleted, err := s.HardDeleteOrganization(ctx, actor, org, slug)
	if err != nil {
		t.Fatal(err)
	}
	if deleted.DryRun || deleted.TotalRows != dry.TotalRows {
		t.Errorf("dry run and delete counts differ: %+v vs %+v", dry, deleted)
	}
	for _, table := range tables {
		var count int64
		if err := db.Table(table).Where("organization_id = ?", org).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("%s retained %d organization rows", table, count)
		}
	}
	for _, table := range []string{"hard_delete_test_children", "hard_delete_test_grandchildren", "password_recovery"} {
		var count int64
		if err := db.Table(table).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("indirect %s retained %d rows", table, count)
		}
	}
	for _, u := range []uuid.UUID{shared, admin} {
		var count int64
		if err := db.Table("users").Where("id = ?", u).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Errorf("shared/platform user %s was deleted", u)
		}
	}
	for _, u := range []uuid.UUID{owner, member} {
		var count int64
		if err := db.Table("users").Where("id = ?", u).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("exclusive user %s was retained", u)
		}
	}
	for table, want := range map[string]int64{"organizations": 1, "organization_memberships": 1, "background_jobs": 1} {
		var count int64
		column := "organization_id"
		if table == "organizations" {
			column = "id"
		}
		if err := db.Table(table).Where(fmt.Sprintf("%s = ?", column), other).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != want {
			t.Errorf("other org %s got %d want %d", table, count, want)
		}
	}
}

func TestReadDeleteSchemaCompositeKeys(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery(`SELECT c.relname AS table_name`).WillReturnRows(sqlmock.NewRows([]string{"table_name", "column_name", "is_key"}).
		AddRow("organizations", "id", true).
		AddRow("conversation_sessions", "id", true).
		AddRow("conversation_sessions", "organization_id", false).
		AddRow("journey_execution_leases", "session_id", true).
		AddRow("journey_execution_leases", "organization_id", false).
		AddRow("password_recovery", "user_id", true))
	mock.ExpectQuery(`SELECT f.oid::bigint AS oid`).WillReturnRows(sqlmock.NewRows([]string{"oid", "child", "parent", "child_column", "parent_column", "action", "position"}).
		AddRow(420, "journey_execution_leases", "conversation_sessions", "organization_id", "organization_id", "a", 1).
		AddRow(420, "journey_execution_leases", "conversation_sessions", "session_id", "id", "a", 2).
		AddRow(421, "password_recovery", "users", "user_id", "id", "c", 1))
	tables, fks, err := readDeleteSchema(db)
	if err != nil {
		t.Fatal(err)
	}
	if got := tables["journey_execution_leases"]; !got.Org || len(got.Keys) != 1 || got.Keys[0] != "session_id" {
		t.Fatalf("non-id primary key lost: %+v", got)
	}
	if got := tables["password_recovery"]; got.Org || len(got.Keys) != 1 || got.Keys[0] != "user_id" {
		t.Fatalf("indirect user table lost: %+v", got)
	}
	if len(fks) != 3 || fks[0].OID != 420 || fks[1].OID != 420 || fks[2].OID != 421 || fks[1].Position != 2 {
		t.Fatalf("foreign-key identity or composite position lost: %+v", fks)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestOrganizationHardDeleteConfirmationAndOwnership(t *testing.T) {
	fx := newCommerceFixture(t, 1)
	fx.service.ConfigureOrganizationHardDelete(true, "staging")
	ctx := context.Background()
	if err := fx.db.Exec(`INSERT INTO organization_memberships(id,organization_id,user_id,role,status,is_owner) VALUES (?,?,?,'merchant_admin','active',false)`, uuid.New(), fx.actor.OrganizationID, fx.actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	_, err := fx.service.HardDeleteOrganization(ctx, fx.actor, fx.actor.OrganizationID, "test-merchant")
	requireDeleteStatus(t, err, 403)
	if err := fx.db.Exec(`UPDATE organization_memberships SET is_owner=true WHERE user_id=?`, fx.actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	_, err = fx.service.HardDeleteOrganization(ctx, fx.actor, fx.actor.OrganizationID, "wrong-slug")
	requireDeleteStatus(t, err, 400)
	_, err = fx.service.HardDeleteOrganization(ctx, fx.actor, fx.actor.OrganizationID, "")
	requireDeleteStatus(t, err, 400)
	// An owner bit in a disabled membership must not authorize deletion.
	if err := fx.db.Exec(`UPDATE organization_memberships SET status='disabled' WHERE user_id=?`, fx.actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	_, err = fx.service.HardDeleteOrganization(ctx, fx.actor, fx.actor.OrganizationID, "test-merchant")
	requireDeleteStatus(t, err, 403)
}

func TestOrganizationHardDeletePlatformPrivilegeIsReadFromDatabase(t *testing.T) {
	fx := newCommerceFixture(t, 1)
	fx.service.ConfigureOrganizationHardDelete(true, "staging")
	actor := auth.CurrentUser{ID: uuid.New(), Role: authz.PlatformAdmin}
	_, err := fx.service.HardDeleteOrganization(context.Background(), actor, fx.actor.OrganizationID, "wrong")
	requireDeleteStatus(t, err, 403)
	if err := fx.db.Exec(`INSERT INTO users(id,email,role,status,password_hash) VALUES (?,?,'platform_admin','active','test-only')`, actor.ID, actor.ID.String()+"@example.invalid").Error; err != nil {
		t.Fatal(err)
	}
	actor.Role = authz.Viewer
	_, err = fx.service.HardDeleteOrganization(context.Background(), actor, fx.actor.OrganizationID, "wrong")
	requireDeleteStatus(t, err, 400)
	if !fx.service.OrganizationHardDeleteCapability(context.Background(), actor, fx.actor.OrganizationID) {
		t.Fatal("active platform admin capability should be enabled")
	}
	if err := fx.db.Exec(`UPDATE users SET status='disabled' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	_, err = fx.service.HardDeleteOrganization(context.Background(), actor, fx.actor.OrganizationID, "wrong")
	requireDeleteStatus(t, err, 403)
}

func TestOrganizationHardDeleteOperatorStillRequiresEnvironmentGuards(t *testing.T) {
	for _, tc := range []struct {
		enabled bool
		env     string
	}{{false, "staging"}, {true, "production"}} {
		service := NewOrganizationHardDeleteOperatorService(nil, tc.enabled, tc.env)
		_, err := service.HardDeleteOrganization(context.Background(), auth.CurrentUser{}, uuid.New(), "test", true)
		requireDeleteStatus(t, err, 403)
	}
	// Operator authority must be isolated from the ordinary HTTP service.
	if NewService(nil, SafeTestProvider{}).organizationHardDeleteOperator {
		t.Fatal("ordinary service must not have operator authority")
	}
}

func TestOrganizationHardDeleteOperatorUsesSameConfirmation(t *testing.T) {
	fx := newCommerceFixture(t, 1)
	operator := NewOrganizationHardDeleteOperatorService(fx.db, true, "staging")
	// No owner membership or platform user is needed for the DB operator, but the
	// exact same slug check must run before entering the PostgreSQL deletion plan.
	_, err := operator.HardDeleteOrganization(context.Background(), auth.CurrentUser{}, fx.actor.OrganizationID, "wrong", true)
	requireDeleteStatus(t, err, 400)
	_, err = operator.HardDeleteOrganization(context.Background(), auth.CurrentUser{}, fx.actor.OrganizationID, "test-merchant", true)
	requireDeleteStatus(t, err, 409) // SQLite fixture: only PostgreSQL can delete.
}
