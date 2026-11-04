package whatsapp

import (
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
	"errors"
	"strings"
)

type flowEnvelope struct {
	EncryptedData string `json:"encrypted_flow_data"`
	EncryptedKey  string `json:"encrypted_aes_key"`
	IV            string `json:"initial_vector"`
}
type flowCipher struct{ key, iv []byte }

func flowSignature(body []byte, signature, secret string) bool {
	if secret == "" || !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	supplied, err := hex.DecodeString(strings.TrimPrefix(signature, "sha256="))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(mac.Sum(nil), supplied)
}
func decryptFlow(body []byte, privatePEM string) ([]byte, flowCipher, error) {
	fail := errors.New("Invalid encrypted Flow request")
	var envelope flowEnvelope
	if json.Unmarshal(body, &envelope) != nil {
		return nil, flowCipher{}, fail
	}
	block, _ := pem.Decode([]byte(privatePEM))
	if block == nil {
		return nil, flowCipher{}, fail
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, flowCipher{}, fail
	}
	private, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, flowCipher{}, fail
	}
	encryptedKey, err := base64.StdEncoding.DecodeString(envelope.EncryptedKey)
	if err != nil {
		return nil, flowCipher{}, fail
	}
	aesKey, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, private, encryptedKey, nil)
	if err != nil || len(aesKey) != 16 {
		return nil, flowCipher{}, fail
	}
	iv, err := base64.StdEncoding.DecodeString(envelope.IV)
	if err != nil || len(iv) != 16 {
		return nil, flowCipher{}, fail
	}
	encrypted, err := base64.StdEncoding.DecodeString(envelope.EncryptedData)
	if err != nil {
		return nil, flowCipher{}, fail
	}
	blockCipher, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, flowCipher{}, fail
	}
	gcm, err := cipher.NewGCMWithNonceSize(blockCipher, 16)
	if err != nil {
		return nil, flowCipher{}, fail
	}
	data, err := gcm.Open(nil, iv, encrypted, nil)
	if err != nil {
		return nil, flowCipher{}, fail
	}
	return data, flowCipher{key: aesKey, iv: iv}, nil
}
func (f flowCipher) encrypt(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(f.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCMWithNonceSize(block, len(f.iv))
	if err != nil {
		return "", err
	}
	iv := make([]byte, len(f.iv))
	for i, b := range f.iv {
		iv[i] = b ^ 0xff
	}
	return base64.StdEncoding.EncodeToString(gcm.Seal(nil, iv, data, nil)), nil
}
