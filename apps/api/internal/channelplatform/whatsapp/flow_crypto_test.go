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
	"testing"
)

func TestFlowOfficialEncryptionAndSignature(t *testing.T) {
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		t.Fatal(err)
	}
	privatePEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: raw}))
	key, iv := make([]byte, 16), make([]byte, 16)
	rand.Read(key)
	rand.Read(iv)
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCMWithNonceSize(block, 16)
	original := []byte(`{"version":"3.0","action":"ping"}`)
	encryptedKey, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, &private.PublicKey, key, nil)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(flowEnvelope{EncryptedData: base64.StdEncoding.EncodeToString(gcm.Seal(nil, iv, original, nil)), EncryptedKey: base64.StdEncoding.EncodeToString(encryptedKey), IV: base64.StdEncoding.EncodeToString(iv)})
	mac := hmac.New(sha256.New, []byte("test-secret"))
	mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !flowSignature(body, signature, "test-secret") || flowSignature(body, signature, "wrong") || flowSignature(body, signature, "") {
		t.Fatal("signature authentication")
	}
	decrypted, c, err := decryptFlow(body, privatePEM)
	if err != nil || string(decrypted) != string(original) {
		t.Fatal("OAEP/GCM", err)
	}
	response, err := c.encrypt(map[string]any{"version": "3.0", "data": map[string]any{"status": "active"}})
	if err != nil {
		t.Fatal(err)
	}
	decoded, _ := base64.StdEncoding.DecodeString(response)
	flipped := make([]byte, 16)
	for i, b := range iv {
		flipped[i] = b ^ 0xff
	}
	plaintext, err := gcm.Open(nil, flipped, decoded, nil)
	if err != nil || !json.Valid(plaintext) {
		t.Fatal("flipped IV response", err)
	}
	var bad flowEnvelope
	json.Unmarshal(body, &bad)
	encrypted, _ := base64.StdEncoding.DecodeString(bad.EncryptedData)
	encrypted[0] ^= 1
	bad.EncryptedData = base64.StdEncoding.EncodeToString(encrypted)
	tampered, _ := json.Marshal(bad)
	if _, _, err := decryptFlow(tampered, privatePEM); err == nil {
		t.Fatal("tampered ciphertext accepted")
	}
	if _, _, err := decryptFlow(body, "bad key"); err == nil {
		t.Fatal("bad key accepted")
	}
}
