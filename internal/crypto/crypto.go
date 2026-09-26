package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"
)

var globalKey []byte

func Init(dataDir string) error {
	keyPath := filepath.Join(dataDir, "secret.key")
	if data, err := os.ReadFile(keyPath); err == nil {
		if len(data) == 32 {
			globalKey = data
			return nil
		}
	}

	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return err
	}
	if err := os.WriteFile(keyPath, key, 0600); err != nil {
		return err
	}
	globalKey = key
	return nil
}

// DeriveKey returns a stable key for a given password (used for seeded derivation if needed).
func DeriveKey(password string) []byte {
	h := sha256.Sum256([]byte(password))
	return h[:]
}

func Encrypt(plaintext string) (string, error) {
	return encryptWithKey(globalKey, plaintext)
}

func Decrypt(ciphertext string) (string, error) {
	return decryptWithKey(globalKey, ciphertext)
}

func encryptWithKey(key []byte, plaintext string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func decryptWithKey(key []byte, ciphertext string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}
	plaintext, err := gcm.Open(nil, data[:nonceSize], data[nonceSize:], nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
