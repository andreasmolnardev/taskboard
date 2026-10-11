package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/andreasmolnardev/slopstack/go/backup"
)

const backupKeyFileName = ".backup-encryption-key"

func createBackupKey(dataDir string) (string, error) {
	key, err := backup.CreateKey(dataDir)
	if err != nil && strings.Contains(err.Error(), "encryption key already exists") {
		return "", errors.New("backup encryption key already exists")
	}
	return key, err
}

func importBackupKey(dataDir, encoded string) error {
	if err := backup.ImportKey(dataDir, encoded); err != nil {
		if strings.Contains(err.Error(), "encryption key already exists") {
			return fmt.Errorf("save recovery key: %w", errors.New("key file already exists"))
		}
		return err
	}
	return nil
}

func loadBackupKey(dataDir string) ([]byte, error) {
	key, err := backup.LoadKey(dataDir)
	if err != nil {
		return nil, fmt.Errorf("read backup encryption key (run 'taskboard backup key-init' once): %w", err)
	}
	return key, nil
}

func encryptBackup(key, plaintext []byte) ([]byte, error) {
	return backup.Encrypt(key, plaintext)
}

func decryptBackup(key, encrypted []byte) ([]byte, error) {
	plaintext, err := backup.Decrypt(key, encrypted)
	if err == nil {
		return plaintext, nil
	}
	switch {
	case strings.Contains(err.Error(), "unsupported or invalid encrypted data"):
		return nil, errors.New("unsupported or invalid encrypted backup")
	case strings.Contains(err.Error(), "invalid encrypted data"):
		return nil, errors.New("invalid encrypted backup")
	case strings.Contains(err.Error(), "could not decrypt data"):
		return nil, errors.New("could not decrypt backup; check the encryption key and file integrity")
	default:
		return nil, err
	}
}
