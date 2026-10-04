package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

func createEncryptedArchive(ctx context.Context, pb core.App, key []byte, prefix string) (string, error) {
	plainName := prefix + ".zip"
	settings := pb.Settings()
	configuredS3 := settings.Backups.S3
	settings.Backups.S3.Enabled = false
	defer func() { settings.Backups.S3 = configuredS3 }()

	if err := pb.CreateBackup(ctx, plainName); err != nil {
		return "", err
	}
	localFS, err := pb.NewBackupsFilesystem()
	if err != nil {
		return "", err
	}
	localFS.SetContext(ctx)
	defer localFS.Close()
	defer localFS.Delete(plainName)

	reader, err := localFS.GetReader(plainName)
	if err != nil {
		return "", err
	}
	plain, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if readErr != nil {
		return "", readErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	encrypted, err := encryptBackup(key, plain)
	if err != nil {
		return "", err
	}

	settings.Backups.S3 = configuredS3
	remoteFS, err := pb.NewBackupsFilesystem()
	if err != nil {
		return "", err
	}
	defer remoteFS.Close()
	remoteFS.SetContext(ctx)
	name := prefix + ".zip.enc"
	if err := remoteFS.Upload(encrypted, name); err != nil {
		return "", err
	}
	return name, nil
}

func cleanupTransientBackupArchives(dataDir string) error {
	backupDir := filepath.Join(dataDir, core.LocalBackupsDirName)
	entries, err := os.ReadDir(backupDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() && strings.HasPrefix(name, "slopstack_restore_") && strings.HasSuffix(name, ".zip") {
			if err := os.Remove(filepath.Join(backupDir, name)); err != nil {
				return err
			}
		}
	}
	return nil
}

func restoreEncryptedArchive(ctx context.Context, pb core.App, key []byte, name string) error {
	if !strings.HasSuffix(name, ".enc") {
		return pb.RestoreBackup(ctx, name)
	}
	fsys, err := pb.NewBackupsFilesystem()
	if err != nil {
		return err
	}
	defer fsys.Close()
	fsys.SetContext(ctx)

	reader, err := fsys.GetReader(name)
	if err != nil {
		return err
	}
	ciphertext, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if readErr != nil {
		return readErr
	}
	if closeErr != nil {
		return closeErr
	}
	plain, err := decryptBackup(key, ciphertext)
	if err != nil {
		return err
	}
	plainName := "slopstack_restore_" + time.Now().UTC().Format("20060102_150405_000000000") + ".zip"
	settings := pb.Settings()
	configuredS3 := settings.Backups.S3
	settings.Backups.S3.Enabled = false
	defer func() { settings.Backups.S3 = configuredS3 }()
	localFS, err := pb.NewBackupsFilesystem()
	if err != nil {
		return err
	}
	defer localFS.Close()
	localFS.SetContext(ctx)
	if err := localFS.Upload(plain, plainName); err != nil {
		return err
	}
	defer localFS.Delete(plainName)
	return pb.RestoreBackup(ctx, plainName)
}

type jsonBackup struct {
	Format      string                       `json:"format"`
	Version     int                          `json:"version"`
	CreatedAt   time.Time                    `json:"created_at"`
	Collections map[string][]json.RawMessage `json:"collections"`
}

var jsonBackupCollections = []string{"todos", "events", "lists", "calendars", "address_books", "contacts"}

func createEncryptedJSON(ctx context.Context, pb core.App, key []byte) (string, error) {
	backup := jsonBackup{
		Format:      "slopstack-data-export",
		Version:     1,
		CreatedAt:   time.Now().UTC(),
		Collections: make(map[string][]json.RawMessage, len(jsonBackupCollections)),
	}
	for _, name := range jsonBackupCollections {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		records, err := pb.FindAllRecords(name)
		if err != nil {
			return "", fmt.Errorf("read %s records: %w", name, err)
		}
		backup.Collections[name] = make([]json.RawMessage, 0, len(records))
		for _, record := range records {
			data, err := json.Marshal(record)
			if err != nil {
				return "", fmt.Errorf("encode %s record: %w", name, err)
			}
			backup.Collections[name] = append(backup.Collections[name], data)
		}
	}
	plain, err := json.MarshalIndent(backup, "", "  ")
	if err != nil {
		return "", err
	}
	ciphertext, err := encryptBackup(key, plain)
	if err != nil {
		return "", err
	}
	name := "data_" + time.Now().UTC().Format("20060102_150405_000000000") + ".json.enc"
	fsys, err := pb.NewBackupsFilesystem()
	if err != nil {
		return "", err
	}
	defer fsys.Close()
	fsys.SetContext(ctx)
	if err := fsys.Upload(ciphertext, name); err != nil {
		return "", err
	}
	return name, nil
}
