package app

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const backupKeyFileName = ".backup-encryption-key"

func createBackupKey(dataDir string) (string, error) {
	keyPath := filepath.Join(dataDir, backupKeyFileName)
	if _, err := os.Stat(keyPath); err == nil {
		return "", errors.New("backup encryption key already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("generate backup encryption key: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(key)
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return "", fmt.Errorf("create data directory: %w", err)
	}
	file, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", fmt.Errorf("save backup encryption key: %w", err)
	}
	if _, err := io.WriteString(file, encoded+"\n"); err != nil {
		file.Close()
		os.Remove(keyPath)
		return "", fmt.Errorf("write backup encryption key: %w", err)
	}
	if err := file.Close(); err != nil {
		os.Remove(keyPath)
		return "", fmt.Errorf("close backup encryption key: %w", err)
	}
	return encoded, nil
}

func importBackupKey(dataDir, encoded string) error {
	key, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil || len(key) != 32 {
		return errors.New("recovery key is invalid")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	file, err := os.OpenFile(filepath.Join(dataDir, backupKeyFileName), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("save recovery key: %w", err)
	}
	if _, err := io.WriteString(file, strings.TrimSpace(encoded)+"\n"); err != nil {
		file.Close()
		os.Remove(filepath.Join(dataDir, backupKeyFileName))
		return fmt.Errorf("write recovery key: %w", err)
	}
	if err := file.Close(); err != nil {
		os.Remove(filepath.Join(dataDir, backupKeyFileName))
		return fmt.Errorf("close recovery key: %w", err)
	}
	return nil
}

func loadBackupKey(dataDir string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(dataDir, backupKeyFileName))
	if err != nil {
		return nil, fmt.Errorf("read backup encryption key (run 'slopstack backup key-init' once): %w", err)
	}
	key, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(data)))
	if err != nil || len(key) != 32 {
		return nil, errors.New("backup encryption key is invalid")
	}
	return key, nil
}

func encryptBackup(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return append([]byte("SLBK1"), gcm.Seal(nonce, nonce, plaintext, []byte("slopstack-backup-v1"))...), nil
}

func decryptBackup(key, encrypted []byte) ([]byte, error) {
	if len(encrypted) < 5 || string(encrypted[:5]) != "SLBK1" {
		return nil, errors.New("unsupported or invalid encrypted backup")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	payload := encrypted[5:]
	if len(payload) < gcm.NonceSize() {
		return nil, errors.New("invalid encrypted backup")
	}
	nonce := payload[:gcm.NonceSize()]
	plaintext, err := gcm.Open(nil, nonce, payload[gcm.NonceSize():], []byte("slopstack-backup-v1"))
	if err != nil {
		return nil, errors.New("could not decrypt backup; check the encryption key and file integrity")
	}
	return plaintext, nil
}
