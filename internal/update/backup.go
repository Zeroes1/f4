package update

import (
	"fmt"
	"os"
)

const executableBackupSuffix = ".f4-update-backup"

// BackupExecutable saves the currently installed executable next to it so a
// failed replacement or restart can restore the last working version.
func BackupExecutable() (string, error) {
	executable, err := Executable()
	if err != nil {
		return "", fmt.Errorf("failed to locate executable for backup: %w", err)
	}
	info, err := os.Stat(executable)
	if err != nil {
		return "", fmt.Errorf("failed to stat executable for backup: %w", err)
	}
	data, err := os.ReadFile(executable)
	if err != nil {
		return "", fmt.Errorf("failed to read executable for backup: %w", err)
	}
	backup := executable + executableBackupSuffix
	if err := os.WriteFile(backup, data, info.Mode().Perm()); err != nil {
		return "", fmt.Errorf("failed to write executable backup: %w", err)
	}
	if err := os.Chmod(backup, info.Mode().Perm()); err != nil {
		return "", fmt.Errorf("failed to preserve executable backup permissions: %w", err)
	}
	return backup, nil
}

// RestoreExecutable replaces the installed executable with a backup and then
// removes the backup. The caller uses this while the old process is still
// alive, before handing control to the restored version.
func RestoreExecutable(backup string) error {
	executable, err := Executable()
	if err != nil {
		return fmt.Errorf("failed to locate executable for restore: %w", err)
	}
	if backup == "" {
		return fmt.Errorf("executable backup path is empty")
	}
	if err := os.Remove(executable); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove failed executable: %w", err)
	}
	if err := os.Rename(backup, executable); err != nil {
		return fmt.Errorf("failed to restore executable backup: %w", err)
	}
	return nil
}

// RemoveExecutableBackup discards a backup after the new executable has
// started successfully or when the user chooses to postpone the restart.
func RemoveExecutableBackup(backup string) error {
	if backup == "" {
		return nil
	}
	if err := os.Remove(backup); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove executable backup: %w", err)
	}
	return nil
}
