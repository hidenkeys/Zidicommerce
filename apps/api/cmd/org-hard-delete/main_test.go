package main

import (
	"context"
	"io"
	"testing"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/config"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestArgumentAndConfirmationGuards(t *testing.T) {
	for _, tc := range []struct {
		name               string
		args               []string
		count              int
		wantError, execute bool
	}{
		{name: "default dry run", args: []string{"--all"}, count: 11},
		{name: "explicit dry run", args: []string{"--org", "one", "--dry-run"}, count: 1},
		{name: "repeatable organizations", args: []string{"--org", "one", "--org", "two"}, count: 2},
		{name: "execute exact eleven", args: []string{"--all", "--execute", "--confirm", "DELETE 11 ORGANIZATIONS"}, count: 11, execute: true},
		{name: "execute explicit false", args: []string{"--all", "--dry-run=false", "--execute", "--confirm", "DELETE 11 ORGANIZATIONS"}, count: 11, execute: true},
		{name: "missing selection", wantError: true},
		{name: "ambiguous selection", args: []string{"--all", "--org", "one"}, wantError: true},
		{name: "empty ref", args: []string{"--org", ""}, wantError: true},
		{name: "unknown flag", args: []string{"--all", "--force"}, wantError: true},
		{name: "positional argument", args: []string{"--all", "staging"}, wantError: true},
		{name: "execute without phrase", args: []string{"--all", "--execute"}, wantError: true},
		{name: "false without execute", args: []string{"--all", "--dry-run=false"}, wantError: true},
		{name: "confirm without execute", args: []string{"--all", "--confirm", "DELETE 11 ORGANIZATIONS"}, wantError: true},
		{name: "conflicting dry run", args: []string{"--all", "--execute", "--dry-run=true", "--confirm", "DELETE 11 ORGANIZATIONS"}, wantError: true},
		{name: "wrong count", args: []string{"--all", "--execute", "--confirm", "DELETE 10 ORGANIZATIONS"}, count: 11, wantError: true},
		{name: "phrase must match exactly", args: []string{"--all", "--execute", "--confirm", "delete 11 organizations"}, count: 11, wantError: true},
		{name: "no organizations", args: []string{"--all"}, count: 0, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o, err := parseOptions(tc.args, io.Discard)
			if err == nil {
				err = o.validateConfirmation(tc.count)
			}
			if (err != nil) != tc.wantError {
				t.Fatalf("error=%v wantError=%t", err, tc.wantError)
			}
			if err == nil && (o.execute != tc.execute || o.dryRun == tc.execute) {
				t.Fatalf("unsafe execution mode: %+v", o)
			}
		})
	}
}

func TestEnvironmentGuard(t *testing.T) {
	for _, tc := range []struct {
		env              string
		enabled, allowed bool
	}{
		{"staging", true, true}, {"development", true, true}, {"production", true, false}, {" Production ", true, false}, {"staging", false, false},
	} {
		err := validateEnvironment(config.Config{AppEnv: tc.env, OrganizationHardDeleteEnabled: tc.enabled})
		if (err == nil) != tc.allowed {
			t.Errorf("env=%q enabled=%t err=%v", tc.env, tc.enabled, err)
		}
	}
}

func TestResolveOrganizationsDeduplicatesExactCount(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := db.Exec("CREATE TABLE organizations (id text PRIMARY KEY, slug text UNIQUE)").Error; err != nil {
		t.Fatal(err)
	}
	id, other := uuid.New(), uuid.New()
	if err := db.Exec("INSERT INTO organizations VALUES (?, 'one'), (?, 'two')", id, other).Error; err != nil {
		t.Fatal(err)
	}
	targets, err := resolveOrganizations(context.Background(), db, options{orgs: orgFlags{id.String(), "one"}})
	if err != nil || len(targets) != 1 || targets[0].ID != id {
		t.Fatalf("deduplication failed: %v %v", targets, err)
	}
	targets, err = resolveOrganizations(context.Background(), db, options{all: true})
	if err != nil || len(targets) != 2 {
		t.Fatalf("all selection failed: %v %v", targets, err)
	}
	if _, err := resolveOrganizations(context.Background(), db, options{orgs: orgFlags{"one", "missing"}}); err == nil {
		t.Fatal("missing selection must stop before any deletion")
	}
	// A UUID-looking slug could identify a different row; refuse ambiguity.
	if err := db.Exec("UPDATE organizations SET slug=? WHERE id=?", id.String(), other).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := resolveOrganizations(context.Background(), db, options{orgs: orgFlags{id.String()}}); err == nil {
		t.Fatal("ambiguous UUID/slug must be refused")
	}
}
