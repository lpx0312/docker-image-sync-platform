package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
)

// encryptionKeyBytes matches internal/services/encryption.NewEncryptionService key derivation.
func encryptionKeyBytes() ([]byte, error) {
	if envKey := os.Getenv("ENCRYPTION_KEY"); envKey != "" {
		h := sha256.Sum256([]byte(envKey))
		return h[:], nil
	}
	if ginMode := os.Getenv("GIN_MODE"); ginMode == "release" {
		return nil, fmt.Errorf("生产环境必须设置 ENCRYPTION_KEY 环境变量")
	}
	defaultKey := "docker-sync-platform-default-key-2024"
	h := sha256.Sum256([]byte(defaultKey))
	return h[:], nil
}

// DecryptSystemConfigValue decrypts values stored with ENC: prefix (same format as EncryptionService).
func DecryptSystemConfigValue(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	if !strings.HasPrefix(ciphertext, "ENC:") {
		return ciphertext, nil
	}
	key, err := encryptionKeyBytes()
	if err != nil {
		return "", err
	}
	encoded := strings.TrimPrefix(ciphertext, "ENC:")
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode base64: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("gcm: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}
	nonce, cipherData := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, cipherData, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plaintext), nil
}
