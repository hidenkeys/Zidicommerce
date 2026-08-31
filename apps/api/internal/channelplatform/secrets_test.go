package channelplatform

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestEncryptedChannelSecretIsScopedAndNotStoredInPlaintext(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&ProviderSecret{}); err != nil {
		t.Fatal(err)
	}
	store, err := NewEncryptedSecretStore(db, []byte("12345678901234567890123456789012"), "test")
	if err != nil {
		t.Fatal(err)
	}
	organizationID := uuid.New()
	connectionID := uuid.New()
	reference, err := store.Save(context.Background(), organizationID, connectionID, "access_token", "private-provider-token")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(reference, "dbenc://") {
		t.Fatalf("expected encrypted reference, got %q", reference)
	}
	resolver := NewDatabaseSecretResolver(db, store)
	resolved, err := resolver.Resolve(context.Background(), SecretScope{OrganizationID: organizationID, ConnectionID: connectionID, CredentialType: "access_token", Reference: reference})
	if err != nil || resolved != "private-provider-token" {
		t.Fatal("encrypted secret did not round trip")
	}
	if _, err := resolver.Resolve(context.Background(), SecretScope{OrganizationID: uuid.New(), ConnectionID: connectionID, CredentialType: "access_token", Reference: reference}); err == nil {
		t.Fatal("cross-tenant secret resolution must fail")
	}
	var stored ProviderSecret
	if err := db.First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored.Ciphertext, "private-provider-token") {
		t.Fatal("secret was stored in plaintext")
	}
}

func TestIncrementMetricAddsRatherThanReplaces(t *testing.T) {
	fx := newChannelFixture(t)
	connection := createConnection(t, fx, fx.actor, "whatsapp", OwnershipMerchantManaged)
	for _, input := range []MetricInput{
		{OrganizationID: fx.actor.OrganizationID, ConnectionID: connection.ID, InboundCount: 1, DeliveredCount: 2},
		{OrganizationID: fx.actor.OrganizationID, ConnectionID: connection.ID, InboundCount: 3, ReadCount: 1},
	} {
		if _, err := fx.service.IncrementMetric(context.Background(), input); err != nil {
			t.Fatal(err)
		}
	}
	summary, err := fx.service.GetMetricsSummary(context.Background(), fx.actor, connection.ID, fx.service.now().AddDate(0, 0, -1), fx.service.now())
	if err != nil {
		t.Fatal(err)
	}
	if summary.InboundCount != 4 || summary.DeliveredCount != 2 || summary.ReadCount != 1 {
		t.Fatalf("unexpected cumulative metrics: %+v", summary)
	}
}
