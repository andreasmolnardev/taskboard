package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/andreasmolnardev/slopstack/go/backup"
	"github.com/pocketbase/pocketbase/core"
)

func createEncryptedArchive(ctx context.Context, pb core.App, prefix string) (string, error) {
	return backup.CreateEncryptedArchive(ctx, pb.DataDir(), backup.PocketBaseStore{App: pb}, "scheduled_", prefix)
}

func cleanupTransientBackupArchives(dataDir string) error {
	return backup.CleanupRestoreArchives(dataDir, "slopstack_restore_")
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
