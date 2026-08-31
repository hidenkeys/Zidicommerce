package channelplatform

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SecretScope struct {
	OrganizationID uuid.UUID
	ConnectionID   uuid.UUID
	CredentialType string
	Reference      string
}

type SecretResolver interface {
	Resolve(context.Context, SecretScope) (string, error)
}

type ProviderSecret struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"-"`
	OrganizationID uuid.UUID `gorm:"type:uuid;index;uniqueIndex:idx_channel_provider_secret" json:"-"`
	ConnectionID   uuid.UUID `gorm:"column:channel_connection_id;type:uuid;index;uniqueIndex:idx_channel_provider_secret" json:"-"`
	CredentialType string    `gorm:"uniqueIndex:idx_channel_provider_secret" json:"-"`
	Ciphertext     string    `json:"-"`
	Nonce          string    `json:"-"`
	KeyVersion     string    `json:"-"`
	CreatedAt      time.Time `json:"-"`
	UpdatedAt      time.Time `json:"-"`
}

func (ProviderSecret) TableName() string { return "channel_provider_secrets" }

type EncryptedSecretStore struct {
	db         *gorm.DB
	key        []byte
	keyVersion string
}

func DecodeChannelSecretKey(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if decoded, err := base64.StdEncoding.DecodeString(raw); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if decoded, err := hex.DecodeString(raw); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if len(raw) == 32 {
		return []byte(raw), nil
	}
	return nil, errors.New("CHANNEL_SECRET_ENCRYPTION_KEY must be 32 raw bytes, 32-byte hex, or 32-byte base64")
}

func NewEncryptedSecretStore(db *gorm.DB, key []byte, keyVersion string) (*EncryptedSecretStore, error) {
	if len(key) != 32 {
		return nil, errors.New("channel secret encryption key must be 32 bytes")
	}
	if strings.TrimSpace(keyVersion) == "" {
		keyVersion = "v1"
	}
	return &EncryptedSecretStore{db: db, key: append([]byte(nil), key...), keyVersion: keyVersion}, nil
}

func (s *EncryptedSecretStore) Save(ctx context.Context, organizationID, connectionID uuid.UUID, credentialType, value string) (string, error) {
	credentialType = normalizeKey(credentialType)
	value = strings.TrimSpace(value)
	if organizationID == uuid.Nil || connectionID == uuid.Nil || credentialType == "" || value == "" {
		return "", errors.New("channel secret requires organization, connection, credential type, and value")
	}
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(value), secretAAD(organizationID, connectionID, credentialType))
	record := ProviderSecret{ID: uuid.New(), OrganizationID: organizationID, ConnectionID: connectionID, CredentialType: credentialType, Ciphertext: base64.StdEncoding.EncodeToString(ciphertext), Nonce: base64.StdEncoding.EncodeToString(nonce), KeyVersion: s.keyVersion}
	err = s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "organization_id"}, {Name: "channel_connection_id"}, {Name: "credential_type"}},
		DoUpdates: clause.AssignmentColumns([]string{"ciphertext", "nonce", "key_version", "updated_at"}),
	}).Create(&record).Error
	if err != nil {
		return "", err
	}
	var stored ProviderSecret
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND channel_connection_id = ? AND credential_type = ?", organizationID, connectionID, credentialType).First(&stored).Error; err != nil {
		return "", err
	}
	return "dbenc://" + stored.ID.String(), nil
}

func (s *EncryptedSecretStore) resolve(ctx context.Context, scope SecretScope, id uuid.UUID) (string, error) {
	var record ProviderSecret
	if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ? AND channel_connection_id = ? AND credential_type = ?", id, scope.OrganizationID, scope.ConnectionID, normalizeKey(scope.CredentialType)).First(&record).Error; err != nil {
		return "", err
	}
	nonce, err := base64.StdEncoding.DecodeString(record.Nonce)
	if err != nil {
		return "", err
	}
	ciphertext, err := base64.StdEncoding.DecodeString(record.Ciphertext)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, secretAAD(scope.OrganizationID, scope.ConnectionID, normalizeKey(scope.CredentialType)))
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

type DatabaseSecretResolver struct {
	db        *gorm.DB
	encrypted *EncryptedSecretStore
}

func NewDatabaseSecretResolver(db *gorm.DB, encrypted *EncryptedSecretStore) *DatabaseSecretResolver {
	return &DatabaseSecretResolver{db: db, encrypted: encrypted}
}

func (r *DatabaseSecretResolver) Resolve(ctx context.Context, scope SecretScope) (string, error) {
	reference := strings.TrimSpace(scope.Reference)
	scheme, value, found := strings.Cut(reference, "://")
	if !found || strings.TrimSpace(value) == "" {
		return "", errors.New("invalid channel secret reference")
	}
	switch strings.ToLower(scheme) {
	case "dbenc":
		if r.encrypted == nil {
			return "", errors.New("encrypted channel secret storage is not configured")
		}
		id, err := uuid.Parse(value)
		if err != nil {
			return "", errors.New("invalid encrypted channel secret reference")
		}
		return r.encrypted.resolve(ctx, scope, id)
	case "env":
		if !regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,127}$`).MatchString(value) {
			return "", errors.New("invalid environment secret reference")
		}
		secret := strings.TrimSpace(os.Getenv(value))
		if secret == "" {
			return "", fmt.Errorf("environment secret %s is not configured", value)
		}
		return secret, nil
	case "legacy":
		return r.resolveLegacy(ctx, scope, value)
	case "vault", "secret", "kms":
		return "", fmt.Errorf("channel secret backend %s is not configured", scheme)
	default:
		return "", errors.New("unsupported channel secret reference")
	}
}

func (r *DatabaseSecretResolver) resolveLegacy(ctx context.Context, scope SecretScope, value string) (string, error) {
	parts := strings.Split(strings.Trim(value, "/"), "/")
	if len(parts) != 3 || parts[0] != "channels" || parts[1] != scope.ConnectionID.String() {
		return "", errors.New("legacy channel secret reference does not match its connection")
	}
	key := normalizeKey(parts[2])
	if key != normalizeKey(scope.CredentialType) {
		return "", errors.New("legacy channel secret reference does not match its credential type")
	}
	var row struct {
		OrganizationID uuid.UUID
		Config         string
		SecretConfig   string
	}
	if err := r.db.WithContext(ctx).Table("channels").Select("organization_id, config, secret_config").Where("id = ? AND organization_id = ?", scope.ConnectionID, scope.OrganizationID).First(&row).Error; err != nil {
		return "", err
	}
	values := map[string]any{}
	if key == "verify_token" {
		_ = json.Unmarshal([]byte(row.Config), &values)
	}
	if strings.TrimSpace(secretString(values[key])) == "" {
		values = map[string]any{}
		_ = json.Unmarshal([]byte(row.SecretConfig), &values)
	}
	secret := strings.TrimSpace(secretString(values[key]))
	if secret == "" {
		return "", errors.New("legacy channel credential is missing")
	}
	return secret, nil
}

func secretAAD(organizationID, connectionID uuid.UUID, credentialType string) []byte {
	return []byte(organizationID.String() + ":" + connectionID.String() + ":" + credentialType)
}

func secretString(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}
