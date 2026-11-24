// org-hard-delete is a maintenance command for trusted database operators.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/config"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/database"
	"github.com/joho/godotenv"
	"gorm.io/gorm"
)

type orgFlags []string

func (v *orgFlags) String() string { return strings.Join(*v, ",") }
func (v *orgFlags) Set(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("--org must be a nonempty ID or slug")
	}
	*v = append(*v, value)
	return nil
}

type options struct {
	orgs                 orgFlags
	all, dryRun, execute bool
	confirm              string
}

func parseOptions(args []string, output io.Writer) (options, error) {
	o := options{dryRun: true}
	flags := flag.NewFlagSet("org-hard-delete", flag.ContinueOnError)
	flags.SetOutput(output)
	flags.Var(&o.orgs, "org", "organization ID or exact slug (repeatable)")
	flags.BoolVar(&o.all, "all", false, "select all organizations present at command start")
	flags.BoolVar(&o.dryRun, "dry-run", true, "print counts without deleting (default true)")
	flags.BoolVar(&o.execute, "execute", false, "delete; requires exact --confirm phrase")
	flags.StringVar(&o.confirm, "confirm", "", "DELETE <N> ORGANIZATIONS, with exact selected count")
	if err := flags.Parse(args); err != nil {
		return o, err
	}
	if flags.NArg() != 0 {
		return o, errors.New("unexpected positional arguments")
	}
	if o.all == (len(o.orgs) > 0) {
		return o, errors.New("select either --all or one or more --org flags")
	}
	explicitDryRun := false
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "dry-run" {
			explicitDryRun = true
		}
	})
	if o.execute {
		if explicitDryRun && o.dryRun {
			return o, errors.New("--execute conflicts with --dry-run=true")
		}
		if o.confirm == "" {
			return o, errors.New("--execute requires --confirm \"DELETE <N> ORGANIZATIONS\"")
		}
		o.dryRun = false
	} else {
		if !o.dryRun {
			return o, errors.New("--dry-run=false requires --execute and --confirm")
		}
		if o.confirm != "" {
			return o, errors.New("--confirm requires --execute")
		}
	}
	return o, nil
}

func (o options) validateConfirmation(count int) error {
	if count == 0 {
		return errors.New("no organizations selected")
	}
	if !o.execute {
		if !o.dryRun {
			return errors.New("deletion requires --execute")
		}
		return nil
	}
	expected := fmt.Sprintf("DELETE %d ORGANIZATIONS", count)
	if o.confirm != expected {
		return fmt.Errorf("confirmation mismatch: requires --confirm %q", expected)
	}
	return nil
}

func validateEnvironment(cfg config.Config) error {
	if strings.EqualFold(strings.TrimSpace(cfg.AppEnv), "production") {
		return errors.New("organization hard delete is unavailable in production")
	}
	if !cfg.OrganizationHardDeleteEnabled {
		return errors.New("ORGANIZATION_HARD_DELETE_ENABLED=true is required")
	}
	return nil
}

type organizationTarget struct {
	ID   uuid.UUID
	Slug string
}

func resolveOrganizations(ctx context.Context, db *gorm.DB, o options) ([]organizationTarget, error) {
	var targets []organizationTarget
	if o.all {
		err := db.WithContext(ctx).Table("organizations").Select("id, slug").Order("id").Find(&targets).Error
		return targets, err
	}
	seen := map[uuid.UUID]bool{}
	for _, ref := range o.orgs {
		var matches []organizationTarget
		query := db.WithContext(ctx).Table("organizations").Select("id, slug")
		if id, err := uuid.Parse(ref); err == nil {
			query = query.Where("id = ? OR slug = ?", id, ref)
		} else {
			query = query.Where("slug = ?", ref)
		}
		if err := query.Find(&matches).Error; err != nil {
			return nil, err
		}
		if len(matches) != 1 {
			return nil, fmt.Errorf("--org %q must resolve to exactly one organization (matched %d)", ref, len(matches))
		}
		if !seen[matches[0].ID] {
			targets = append(targets, matches[0])
			seen[matches[0].ID] = true
		}
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].ID.String() < targets[j].ID.String() })
	return targets, nil
}

// Both paths call the HTTP service's exact deletion method; no deletion SQL lives
// in the CLI. Each call owns its transaction, so errors stop the batch immediately.
func run(ctx context.Context, args []string, output, errorOutput io.Writer) error {
	o, err := parseOptions(args, errorOutput)
	if err != nil {
		return err
	}
	_ = godotenv.Load()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := validateEnvironment(cfg); err != nil {
		return err
	}
	slog.SetDefault(config.NewLogger(cfg.LogLevel))
	db, sqlDB, err := database.Connect(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	targets, err := resolveOrganizations(ctx, db, o)
	if err != nil {
		return err
	}
	if err := o.validateConfirmation(len(targets)); err != nil {
		return err
	}
	service := core.NewOrganizationHardDeleteOperatorService(db, cfg.OrganizationHardDeleteEnabled, cfg.AppEnv)
	fmt.Fprintf(output, "organizations=%d dry_run=%t\n", len(targets), o.dryRun)
	for _, org := range targets {
		result, err := service.HardDeleteOrganization(ctx, auth.CurrentUser{}, org.ID, org.Slug, o.dryRun)
		if err != nil {
			return fmt.Errorf("organization %s: %w (batch stopped; earlier executed organizations remain deleted)", org.ID, err)
		}
		fmt.Fprintf(output, "organization_id=%s slug=%q total_rows=%d dry_run=%t\n", org.ID, org.Slug, result.TotalRows, result.DryRun)
		tables := make([]string, 0, len(result.Counts))
		for table := range result.Counts {
			tables = append(tables, table)
		}
		sort.Strings(tables)
		for _, table := range tables {
			fmt.Fprintf(output, "  %s=%d\n", table, result.Counts[table])
		}
	}
	return nil
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
