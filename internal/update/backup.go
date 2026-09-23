package update

import (
	"fmt"
	"os"
)

// BackupExecutable saves the currently installed executable in a temporary
// file so protected installation directories do not prevent an update.
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
	tmp, err := os.CreateTemp("", "f4-update-backup-*")
	if err != nil {
		return "", fmt.Errorf("failed to create executable backup: %w", err)
	}
	backup := tmp.Name()
	removeBackup := true
	defer func() {
		if removeBackup {
			_ = os.Remove(backup)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("failed to write executable backup: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("failed to close executable backup: %w", err)
	}
	if err := os.Chmod(backup, info.Mode().Perm()); err != nil {
		return "", fmt.Errorf("failed to preserve executable backup permissions: %w", err)
	}
	removeBackup = false
	return backup, nil
}

// RestoreExecutable replaces the installed executable with a backup and then
// removes the backup. The caller uses this while the old process is still
// alive, before handing control to the restored version.
func RestoreExecutable(backup string) error {
	if err := restoreExecutable(backup); err != nil {
		if !isPermissionError(err) {
			return err
		}
		if elevatedErr := restoreExecutableElevated(backup); elevatedErr != nil {
			return fmt.Errorf("failed to restore executable directly: %v; elevated restore failed: %w", err, elevatedErr)
		}
	}
	return nil
}

func restoreExecutable(backup string) error {
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

// RunRestoreHelper is used by the elevated Windows helper. It deliberately
// calls the direct implementation so a failed permission check cannot recurse
// into another elevation request.
var RunRestoreHelper = restoreExecutable

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
