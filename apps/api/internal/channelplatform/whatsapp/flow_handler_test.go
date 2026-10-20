package whatsapp

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/channelplatform"
	runtimeengine "github.com/hidenkeys/zidicommerce/apps/api/internal/runtime"
)

type flowHandlerProcessor struct {
	organization, connection uuid.UUID
	calls                    int
}

func (p *flowHandlerProcessor) ProcessChannelInbound(context.Context, uuid.UUID, channelplatform.InboundEvent) error {
	return nil
}
func (p *flowHandlerProcessor) ExchangeJourneyFlow(_ context.Context, org, connection uuid.UUID, request runtimeengine.JourneyFlowRequest) (map[string]any, error) {
	p.organization, p.connection, p.calls = org, connection, p.calls+1
	return map[string]any{"version": "3.0", "data": map[string]any{"status": "active"}}, nil
}

func TestJourneyFlowHTTPRequiresSignatureEncryptionAndSandbox(t *testing.T) {
	fx := newWhatsAppFixture(t)
	configureWhatsApp(t, fx)
	fx.service.ConfigureFlows(FlowConfig{Enabled: true, TestWABAID: "waba-1", TestPhoneID: "1234567890"}, nil)
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKCS8PrivateKey(private)
	keyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	if err := fx.service.saveCredentialInput(context.Background(), fx.actor, fx.connection.ID, flowPrivateKey, nil, &keyPEM); err != nil {
		t.Fatal(err)
	}
	key, iv := make([]byte, 16), make([]byte, 16)
	rand.Read(key)
	rand.Read(iv)
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCMWithNonceSize(block, 16)
	encryptedKey, _ := rsa.EncryptOAEP(sha256.New(), rand.Reader, &private.PublicKey, key, nil)
	body, _ := json.Marshal(flowEnvelope{EncryptedData: base64.StdEncoding.EncodeToString(gcm.Seal(nil, iv, []byte(`{"version":"3.0","action":"ping"}`), nil)), EncryptedKey: base64.StdEncoding.EncodeToString(encryptedKey), IV: base64.StdEncoding.EncodeToString(iv)})
	mac := hmac.New(sha256.New, []byte("app-secret"))
	mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	processor := &flowHandlerProcessor{}
	app := fiber.New()
	NewHandler(fx.service, fx.platform, fx.adapter, processor).RegisterPublic(app.Group("/v1"))
	send := func(sig string) (int, []byte) {
		t.Helper()
		req := httptest.NewRequest("POST", "/v1/journey-flows/"+fx.connection.ID.String()+"/data", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Hub-Signature-256", sig)
		response, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		raw, _ := io.ReadAll(response.Body)
		return response.StatusCode, raw
	}
	if status, _ := send(""); status != 432 || processor.calls != 0 {
		t.Fatal("unsigned request accepted", status)
	}
	status, response := send(signature)
	if status != 200 || processor.calls != 1 || processor.organization != fx.actor.OrganizationID || processor.connection != fx.connection.ID {
		t.Fatal("signed encrypted request resolution", status)
	}
	decoded, err := base64.StdEncoding.DecodeString(string(response))
	if err != nil {
		t.Fatal(err)
	}
	for i := range iv {
		iv[i] ^= 0xff
	}
	plain, err := gcm.Open(nil, iv, decoded, nil)
	if err != nil || !json.Valid(plain) || bytes.Contains(response, []byte("app-secret")) {
		t.Fatal("encrypted response", err)
	}
	fx.db.Model(&channelplatform.ChannelConnection{}).Where("id = ?", fx.connection.ID).Update("environment", channelplatform.EnvironmentProduction)
	if status, _ := send(signature); status != 403 || processor.calls != 1 {
		t.Fatal("production asset reached Flow engine", status)
	}
	fx.service.flowConfig.Enabled = false
	if status, _ := send(signature); status != 503 {
		t.Fatal("disabled Flow endpoint accepted", status)
	}
}

func TestJourneyFlowAuditDoesNotStoreArbitraryRequestText(t *testing.T) {
	for _, action := range []string{"ping", "INIT", "BACK", "data_exchange"} {
		if got := flowAuditAction(action); got != action {
			t.Fatalf("known action %q classified as %q", action, got)
		}
	}
	for _, action := range []string{"", "customer answer with a secret", "data_exchange\nflow_token=private"} {
		if got := flowAuditAction(action); got != "invalid_action" {
			t.Fatalf("untrusted action text persisted as %q", got)
		}
	}
}
