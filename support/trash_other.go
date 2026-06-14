//go:build !windows

package support

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// moveToRecycleBin moves the file at path to the OS trash directory.
// On macOS it moves the file to ~/.Trash; on Linux it follows the
// FreeDesktop.org trash specification.
func MoveToRecycleBin(path string) error {
	switch runtime.GOOS {
	case "darwin":
		return moveToMacTrash(path)
	default:
		return moveToFreedesktopTrash(path)
	}
}

func moveToMacTrash(path string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	trashDir := filepath.Join(home, ".Trash")
	if err := os.MkdirAll(trashDir, 0700); err != nil {
		return err
	}
	dest := uniqueTrashDest(trashDir, filepath.Base(path))
	return os.Rename(path, dest)
}

func moveToFreedesktopTrash(path string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	filesDir := filepath.Join(home, ".local", "share", "Trash", "files")
	infoDir := filepath.Join(home, ".local", "share", "Trash", "info")
	if err := os.MkdirAll(filesDir, 0700); err != nil {
		return err
	}
	if err := os.MkdirAll(infoDir, 0700); err != nil {
		return err
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	dest := uniqueTrashDest(filesDir, filepath.Base(path))
	destBase := filepath.Base(dest)

	// Write .trashinfo before renaming so the file is never without metadata.
	infoPath := filepath.Join(infoDir, destBase+".trashinfo")
	infoContent := fmt.Sprintf(
		"[Trash Info]\nPath=%s\nDeletionDate=%s\n",
		absPath,
		time.Now().Format("2006-01-02T15:04:05"),
	)
	if err := os.WriteFile(infoPath, []byte(infoContent), 0600); err != nil {
		return err
	}
	if err := os.Rename(path, dest); err != nil {
		_ = os.Remove(infoPath) // clean up orphaned .trashinfo on failure
		return err
	}
	return nil
}

// uniqueTrashDest returns a destination path inside dir for a file named base,
// appending a counter suffix if a file with that name already exists.
func uniqueTrashDest(dir, base string) string {
	dest := filepath.Join(dir, base)
	ext := filepath.Ext(base)
	stem := base[:len(base)-len(ext)]
	for i := 1; ; i++ {
		if _, err := os.Lstat(dest); os.IsNotExist(err) {
			return dest
		}
		dest = filepath.Join(dir, fmt.Sprintf("%s_%d%s", stem, i, ext))
	}
}
