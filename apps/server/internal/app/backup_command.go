package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/spf13/cobra"
)

var backupNamePattern = regexp.MustCompile(`^@?[a-z0-9_-]+\.zip(\.enc)?$`)

const backupRestoreRestartedEnv = "SLOPSTACK_BACKUP_RESTORE_RESTARTED"

type BackupFile struct {
	Name     string
	Size     int64
	Modified time.Time
}

type BackupCommand struct {
	dataDir    string
	Create     func(context.Context) (string, error)
	List       func(context.Context) ([]BackupFile, error)
	Restore    func(context.Context, string) error
	createJSON func(context.Context) (string, error)
}

func NewBackupCommand(pb core.App) *BackupCommand {
	return &BackupCommand{
		dataDir: pb.DataDir(),
		createJSON: func(ctx context.Context) (string, error) {
			key, err := loadBackupKey(pb.DataDir())
			if err != nil {
				return "", err
			}
			return createEncryptedJSON(ctx, pb, key)
		},
		Create: func(ctx context.Context) (string, error) {
			key, err := loadBackupKey(pb.DataDir())
			if err != nil {
				return "", err
			}
			return createEncryptedArchive(ctx, pb, key, "manual_"+time.Now().UTC().Format("20060102_150405_000000000"))
		},
		List: func(ctx context.Context) ([]BackupFile, error) {
			fsys, err := pb.NewBackupsFilesystem()
			if err != nil {
				return nil, err
			}
			defer fsys.Close()
			fsys.SetContext(ctx)

			objects, err := fsys.List("")
			if err != nil {
				return nil, err
			}
			files := make([]BackupFile, 0, len(objects))
			for _, object := range objects {
				if object.IsDir {
					continue
				}
				files = append(files, BackupFile{Name: object.Key, Size: object.Size, Modified: object.ModTime})
			}
			sort.Slice(files, func(i, j int) bool { return files[i].Modified.After(files[j].Modified) })
			return files, nil
		},
		Restore: func(ctx context.Context, name string) error {
			if !strings.HasSuffix(name, ".enc") {
				return pb.RestoreBackup(ctx, name)
			}
			key, err := loadBackupKey(pb.DataDir())
			if err != nil {
				return err
			}
			return restoreEncryptedArchive(ctx, pb, key, name)
		},
	}
}

func (c *BackupCommand) Command() *cobra.Command {
	backup := &cobra.Command{Use: "backup", Short: "Manage PocketBase backups"}
	backup.AddCommand(c.createCommand(), c.jsonCommand(), c.keyInitCommand(), c.keyImportCommand(), c.listCommand(), c.restoreCommand())
	return backup
}

func (c *BackupCommand) createCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "create",
		Short: "Create a manual backup",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			name, err := c.Create(cmd.Context())
			if err != nil {
				return fmt.Errorf("create backup: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), name)
			return nil
		},
	}
}

func (c *BackupCommand) keyInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "key-init",
		Short: "Create a backup encryption key and show it once",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			key, err := createBackupKey(c.dataDir)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Save this recovery key now. It is shown only once:")
			fmt.Fprintln(cmd.OutOrStdout(), key)
			return nil
		},
	}
}

func (c *BackupCommand) keyImportCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "key-import",
		Short: "Install a saved recovery key from standard input",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			value, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), 1025))
			if err != nil {
				return err
			}
			if len(value) > 1024 {
				return errors.New("recovery key input is too large")
			}
			if err := importBackupKey(c.dataDir, string(value)); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Recovery key installed.")
			return nil
		},
	}
}

func (c *BackupCommand) jsonCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "json",
		Short: "Create an encrypted JSON export of taskboard records",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			name, err := c.createJSON(cmd.Context())
			if err != nil {
				return fmt.Errorf("create JSON backup: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), name)
			return nil
		},
	}
}

func (c *BackupCommand) listCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List backup archives",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			files, err := c.List(cmd.Context())
			if err != nil {
				return fmt.Errorf("list backups: %w", err)
			}
			return writeBackupList(cmd.OutOrStdout(), files)
		},
	}
}

func (c *BackupCommand) restoreCommand() *cobra.Command {
	var confirmed bool
	command := &cobra.Command{
		Use:   "restore <exact-name>",
		Short: "Restore a backup and restart the process",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := validateBackupName(name); err != nil {
				return err
			}
			if !confirmed {
				return errors.New("restore requires --confirm")
			}
			if os.Getenv(backupRestoreRestartedEnv) == "1" {
				return nil
			}
			if err := os.Setenv(backupRestoreRestartedEnv, "1"); err != nil {
				return fmt.Errorf("prepare backup restore restart: %w", err)
			}
			defer func() { _ = os.Unsetenv(backupRestoreRestartedEnv) }()
			if err := c.Restore(cmd.Context(), name); err != nil {
				return fmt.Errorf("restore backup %q: %w", name, err)
			}
			return nil
		},
	}
	command.Flags().BoolVar(&confirmed, "confirm", false, "confirm destructive restore")
	return command
}

func validateBackupName(name string) error {
	if len(name) > 150 || !backupNamePattern.MatchString(name) {
		return fmt.Errorf("unsafe backup name %q: use an exact archive name from 'backup list'", name)
	}
	return nil
}

func writeBackupList(output io.Writer, files []BackupFile) error {
	writer := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "NAME\tSIZE\tMODIFIED"); err != nil {
		return err
	}
	for _, file := range files {
		if _, err := fmt.Fprintf(writer, "%s\t%d\t%s\n", file.Name, file.Size, file.Modified.UTC().Format(time.RFC3339)); err != nil {
			return err
		}
	}
	return writer.Flush()
}
