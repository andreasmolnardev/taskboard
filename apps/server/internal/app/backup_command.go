package app

import (
	"fmt"

	"github.com/andreasmolnardev/slopstack/go/backup"
	"github.com/pocketbase/pocketbase/core"
	"github.com/spf13/cobra"
)

const backupRestoreRestartedEnv = "TASKBOARD_BACKUP_RESTORE_RESTARTED"

func NewBackupCommand(pb core.App) *cobra.Command {
	return backup.NewCommandWithOptions(backup.CommandOptions{
		DataDir:               pb.DataDir(),
		Store:                 backup.PocketBaseStore{App: pb},
		EncryptedCreateOnly:   true,
		KeyImportMode:         backup.KeyImportStdin,
		KeyInitMessage:        "Save this recovery key now. It is shown only once:",
		KeyImportMessage:      "Recovery key installed.",
		RestartGuardEnv:       backupRestoreRestartedEnv,
		LegacyRestartGuardEnv: "SLOPSTACK_BACKUP_RESTORE_RESTARTED",
		CreateTempPrefix:      "manual_",
		RestoreTempPrefix:     "slopstack_restore_",
		AdditionalCommands:    []*cobra.Command{jsonBackupCommand(pb)},
	})
}

func jsonBackupCommand(pb core.App) *cobra.Command {
	return &cobra.Command{
		Use:   "json",
		Short: "Create an encrypted JSON export of taskboard records",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			key, err := loadBackupKey(pb.DataDir())
			if err != nil {
				return err
			}
			name, err := createEncryptedJSON(cmd.Context(), pb, key)
			if err != nil {
				return fmt.Errorf("create JSON backup: %w", err)
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), name)
			return err
		},
	}
}
