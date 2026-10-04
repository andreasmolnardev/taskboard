package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

func TestEncryptedBackupRoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	plain := []byte("backup contents")
	ciphertext, err := encryptBackup(key, plain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext, plain) {
		t.Fatal("ciphertext contains plaintext")
	}
	got, err := decryptBackup(key, ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("decrypted %q, want %q", got, plain)
	}
	if _, err := decryptBackup(bytes.Repeat([]byte{8}, 32), ciphertext); err == nil {
		t.Fatal("expected wrong key to fail")
	}
}

func TestCreateBackupKeyIsOneTimeAndPrivate(t *testing.T) {
	dataDir := t.TempDir()
	key, err := createBackupKey(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := loadBackupKey(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(key) == 0 || len(loaded) != 32 {
		t.Fatalf("expected a 32-byte key, got %q and %d bytes", key, len(loaded))
	}
	info, err := os.Stat(filepath.Join(dataDir, backupKeyFileName))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("key file permissions are %o, want 600", info.Mode().Perm())
	}
	if _, err := createBackupKey(dataDir); err == nil {
		t.Fatal("expected key reinitialization to fail")
	}

	restoredDir := t.TempDir()
	if err := importBackupKey(restoredDir, key); err != nil {
		t.Fatal(err)
	}
	restoredKey, err := loadBackupKey(restoredDir)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(loaded, restoredKey) {
		t.Fatal("imported recovery key does not match")
	}
}

func TestCreateEncryptedJSONExport(t *testing.T) {
	testApp, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer testApp.Cleanup()
	if err := EnsureCollections(testApp); err != nil {
		t.Fatal(err)
	}

	key := bytes.Repeat([]byte{5}, 32)
	name, err := createEncryptedJSON(context.Background(), testApp, key)
	if err != nil {
		t.Fatal(err)
	}
	fsys, err := testApp.NewBackupsFilesystem()
	if err != nil {
		t.Fatal(err)
	}
	defer fsys.Close()
	reader, err := fsys.GetReader(name)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := io.ReadAll(reader)
	reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	plain, err := decryptBackup(key, ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	var exported jsonBackup
	if err := json.Unmarshal(plain, &exported); err != nil {
		t.Fatal(err)
	}
	if exported.Format != "slopstack-data-export" || exported.Version != 1 {
		t.Fatalf("unexpected JSON export metadata: %#v", exported)
	}
	for _, collection := range jsonBackupCollections {
		if _, ok := exported.Collections[collection]; !ok {
			t.Errorf("JSON export is missing %s", collection)
		}
	}
}

func TestCleanupTransientBackupArchives(t *testing.T) {
	dataDir := t.TempDir()
	backupDir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"slopstack_restore_old.zip", "manual.zip", "slopstack_restore_keep.json"} {
		if err := os.WriteFile(filepath.Join(backupDir, name), []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := cleanupTransientBackupArchives(dataDir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(backupDir, "slopstack_restore_old.zip")); !os.IsNotExist(err) {
		t.Fatal("temporary restore archive was not removed")
	}
	for _, name := range []string{"manual.zip", "slopstack_restore_keep.json"} {
		if _, err := os.Stat(filepath.Join(backupDir, name)); err != nil {
			t.Fatalf("unexpected removal of %s: %v", name, err)
		}
	}
}

func TestEncryptedBackupRejectsTampering(t *testing.T) {
	key := bytes.Repeat([]byte{3}, 32)
	ciphertext, err := encryptBackup(key, []byte("data"))
	if err != nil {
		t.Fatal(err)
	}
	ciphertext[len(ciphertext)-1] ^= 0xff
	if _, err := decryptBackup(key, ciphertext); err == nil {
		t.Fatal("expected tampered backup to fail authentication")
	}
}
