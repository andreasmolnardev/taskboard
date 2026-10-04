package app

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase"
)

func registerEncryptedBackupScheduler(pb *pocketbase.PocketBase, config Config) {
	if !config.BackupEnabled {
		return
	}
	if err := pb.Cron().Add("encrypted-backups", config.BackupCron, func() {
		ctx := context.Background()
		key, err := loadBackupKey(pb.DataDir())
		if err != nil {
			pb.Logger().Error("create scheduled backup", "error", err)
			return
		}
		stamp := time.Now().UTC().Format("20060102_150405_000000000")
		name, err := createEncryptedArchive(ctx, pb, key, "scheduled_"+stamp)
		if err != nil {
			pb.Logger().Error("create scheduled backup", "error", err)
		} else {
			pb.Logger().Info("created encrypted backup", "name", name)
		}
		if config.BackupJSONEnabled {
			jsonName, jsonErr := createEncryptedJSON(ctx, pb, key)
			if jsonErr != nil {
				pb.Logger().Error("create scheduled JSON backup", "error", jsonErr)
			} else {
				pb.Logger().Info("created encrypted JSON backup", "name", jsonName)
			}
		}
		if err := pruneEncryptedBackups(ctx, pb, config.BackupMaxKeep); err != nil {
			pb.Logger().Error("prune encrypted backups", "error", err)
		}
	}); err != nil {
		pb.Logger().Error("register encrypted backup schedule", "error", err)
	}
}

func pruneEncryptedBackups(ctx context.Context, pb *pocketbase.PocketBase, keep int) error {
	if keep < 1 {
		return nil
	}
	fsys, err := pb.NewBackupsFilesystem()
	if err != nil {
		return err
	}
	defer fsys.Close()
	fsys.SetContext(ctx)
	objects, err := fsys.List("")
	if err != nil {
		return err
	}
	for _, suffix := range []string{".zip.enc", ".json.enc"} {
		var matching []struct {
			key string
			mod time.Time
		}
		for _, object := range objects {
			if !object.IsDir && strings.HasSuffix(object.Key, suffix) {
				matching = append(matching, struct {
					key string
					mod time.Time
				}{object.Key, object.ModTime})
			}
		}
		sort.Slice(matching, func(i, j int) bool { return matching[i].mod.After(matching[j].mod) })
		if len(matching) <= keep {
			continue
		}
		for _, object := range matching[keep:] {
			if err := fsys.Delete(object.key); err != nil {
				return err
			}
		}
	}
	return nil
}
