package core

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PaymentProviderSecretStore interface {
	SaveSecret(ctx context.Context, organizationID uuid.UUID, provider, name, value string) error
	GetSecret(ctx context.Context, organizationID uuid.UUID, provider, name string) (string, error)
	DeleteSecret(ctx context.Context, organizationID uuid.UUID, provider, name string) error
	HasSecret(ctx context.Context, organizationID uuid.UUID, provider, name string) (bool, error)
}

type EncryptedPaymentSecretStore struct {
	db         *gorm.DB
	key        []byte
	keyVersion string
}

func NewEncryptedPaymentSecretStore(db *gorm.DB, key []byte, keyVersion string) (*EncryptedPaymentSecretStore, error) {
	if len(key) != 32 {
		return nil, errors.New("payment secret encryption key must be 32 bytes")
	}
	if strings.TrimSpace(keyVersion) == "" {
		keyVersion = "v1"
	}
	return &EncryptedPaymentSecretStore{db: db, key: key, keyVersion: keyVersion}, nil
}

func DecodePaymentSecretKey(raw string) ([]byte, error) {
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
	return nil, errors.New("PAYMENT_SECRET_ENCRYPTION_KEY must be 32 raw bytes, 32-byte hex, or 32-byte base64")
}

func (s *EncryptedPaymentSecretStore) SaveSecret(ctx context.Context, organizationID uuid.UUID, provider, name, value string) error {
	provider = strings.ToLower(strings.TrimSpace(provider))
	name = strings.ToLower(strings.TrimSpace(name))
	value = strings.TrimSpace(value)
	if organizationID == uuid.Nil || provider == "" || name == "" || value == "" {
		return errors.New("payment secret requires organization, provider, name, and value")
	}
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(value), []byte(organizationID.String()+":"+provider+":"+name))
	record := PaymentProviderSecret{ID: uuid.New(), OrganizationID: organizationID, Provider: provider, SecretName: name, Ciphertext: base64.StdEncoding.EncodeToString(ciphertext), Nonce: base64.StdEncoding.EncodeToString(nonce), KeyVersion: s.keyVersion}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "organization_id"}, {Name: "provider"}, {Name: "secret_name"}}, DoUpdates: clause.Assignments(map[string]any{"ciphertext": record.Ciphertext, "nonce": record.Nonce, "key_version": record.KeyVersion})}).Create(&record).Error
}

func (s *EncryptedPaymentSecretStore) GetSecret(ctx context.Context, organizationID uuid.UUID, provider, name string) (string, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	name = strings.ToLower(strings.TrimSpace(name))
	var record PaymentProviderSecret
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND provider = ? AND secret_name = ?", organizationID, provider, name).First(&record).Error; err != nil {
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
	plain, err := gcm.Open(nil, nonce, ciphertext, []byte(organizationID.String()+":"+provider+":"+name))
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func (s *EncryptedPaymentSecretStore) DeleteSecret(ctx context.Context, organizationID uuid.UUID, provider, name string) error {
	return s.db.WithContext(ctx).Where("organization_id = ? AND provider = ? AND secret_name = ?", organizationID, strings.ToLower(strings.TrimSpace(provider)), strings.ToLower(strings.TrimSpace(name))).Delete(&PaymentProviderSecret{}).Error
}

func (s *EncryptedPaymentSecretStore) HasSecret(ctx context.Context, organizationID uuid.UUID, provider, name string) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&PaymentProviderSecret{}).Where("organization_id = ? AND provider = ? AND secret_name = ?", organizationID, strings.ToLower(strings.TrimSpace(provider)), strings.ToLower(strings.TrimSpace(name))).Count(&count).Error
	return count > 0, err
}
