// journey-qa-seed creates an isolated, explicitly acknowledged local QA tenant.
// It cannot connect to a remote database or an ordinary database name.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/organization"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"net/url"
	"os"
	"strings"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func main() {
	dsn := os.Getenv("DATABASE_URL")
	u, err := url.Parse(dsn)
	must(err)
	if os.Getenv("JOURNEY_QA_ACKNOWLEDGE") != "local-test-records" || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") || !strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "zidi_r1_qa") {
		panic("Use an explicitly acknowledged loopback zidi_r1_qa database; remote databases are prohibited")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	must(err)
	raw := make([]byte, 12)
	_, err = rand.Read(raw)
	must(err)
	suffix := hex.EncodeToString(raw[:4])
	password := "JourneyQA-" + hex.EncodeToString(raw)
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	must(err)
	owner := organization.User{ID: uuid.New(), Email: "journey.owner." + suffix + "@zidi.test", PasswordHash: string(hash), FirstName: "Journey", LastName: "QA", Role: authz.MerchantAdmin, Status: "active"}
	must(db.Create(&owner).Error)
	ctx := context.Background()
	commerce := core.NewService(db, core.SafeTestProvider{})
	org, _, err := commerce.OnboardOrganization(ctx, auth.CurrentUser{ID: owner.ID, Role: owner.Role}, core.OrganizationInput{Name: "Journey Studio QA " + suffix, Slug: "journey-qa-" + suffix, Country: "NG", Currency: "NGN", Timezone: "Africa/Lagos", Metadata: `{"journey_qa":true}`})
	must(err)
	actor := auth.CurrentUser{ID: owner.ID, OrganizationID: org.ID, Role: authz.MerchantAdmin}
	viewer := organization.User{ID: uuid.New(), OrganizationID: &org.ID, Email: "journey.viewer." + suffix + "@zidi.test", PasswordHash: string(hash), FirstName: "Read", LastName: "Only", Role: authz.Viewer, Status: "active"}
	must(db.Create(&viewer).Error)
	must(db.Create(&organization.OrganizationMembership{ID: uuid.New(), OrganizationID: org.ID, UserID: viewer.ID, Role: authz.Viewer, Status: "active"}).Error)
	channel, err := commerce.CreateChannel(ctx, actor, core.ChannelInput{Provider: "test", DisplayName: "Local journey QA (no external messages)", PhoneNumberID: "qa-" + suffix, Status: "active"})
	must(err)
	storeName := "Journey QA Victoria Island Flagship with a deliberately long accessible store name"
	store, err := commerce.CreateStore(ctx, actor, core.StoreInput{Name: storeName, Code: "QA", Status: "active", Address: "Local test fixture", FulfilmentModes: []core.StoreFulfilmentModeInput{{Mode: "pickup", Enabled: true}}})
	must(err)
	category, err := commerce.CreateCategory(ctx, actor, core.CategoryInput{Name: "Drinks", Slug: "drinks", Status: "active"})
	must(err)
	productName := "Special Jasmine Milk Tea with Coconut Pearls and a deliberately long product description"
	product, err := commerce.CreateProduct(ctx, actor, core.ProductInput{CategoryID: &category.ID, Name: productName, Slug: "jasmine-tea", Status: "active", Variants: []core.VariantInput{{Name: "Regular Jasmine Milk Tea", SKU: "QA-TEA", PriceMinor: 450000, Currency: "NGN", Status: "active"}}})
	must(err)
	_, err = commerce.UpsertInventory(ctx, actor, core.InventoryCreateInput{StoreID: store.ID, VariantID: product.Variants[0].ID, OnHand: 100})
	must(err)
	manifest := map[string]any{"organization_id": org.ID, "owner_email": owner.Email, "viewer_email": viewer.Email, "password": password, "channel_id": channel.ID, "store_name": storeName, "product_name": productName}
	data, err := json.MarshalIndent(manifest, "", "  ")
	must(err)
	path := os.Getenv("JOURNEY_QA_MANIFEST")
	if path == "" {
		path = "/tmp/zidi-r1-qa-manifest.json"
	}
	must(os.WriteFile(path, data, 0600))
	fmt.Println("Local QA tenant created; credentials and fixture IDs are in " + path)
}
