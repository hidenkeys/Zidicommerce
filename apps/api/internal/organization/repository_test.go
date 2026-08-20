package organization

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/tenant"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRepositoryFindByIDUsesOrganizationIDAsTenantRoot(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Organization{}); err != nil {
		t.Fatal(err)
	}

	org := Organization{ID: uuid.New(), Name: "Demo", Slug: "demo", Currency: "NGN", Timezone: "Africa/Lagos", Status: "active", Metadata: "{}"}
	if err := db.Create(&org).Error; err != nil {
		t.Fatal(err)
	}
	scope, err := tenant.NewScope(org.ID)
	if err != nil {
		t.Fatal(err)
	}

	found, err := NewRepository(db).FindByID(context.Background(), scope, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if found.ID != org.ID {
		t.Fatalf("expected organization %s, got %s", org.ID, found.ID)
	}

	otherScope, err := tenant.NewScope(uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRepository(db).FindByID(context.Background(), otherScope, org.ID); err == nil {
		t.Fatal("expected mismatched tenant scope to be rejected")
	}
}
