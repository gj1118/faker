//go:build !windows

package support

import (
	"fmt"
	"log/slog"
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
	slog.Info("MoveToMacTrash:: will move to MacTrash", "path", path)
	home, err := os.UserHomeDir()
	if err != nil {
		slog.Error("MoveToMacTrash:: there was an error encountered while getting the user homedir", "error", err)
		return err
	}
	slog.Info("MoveToMacTrash:: User homedir was obtained", "home", home)
	trashDir := filepath.Join(home, ".Trash")
	slog.Info("MoveToMacTrash:: TrashDir", "trashDir", trashDir)
	if err := os.MkdirAll(trashDir, 0700); err != nil {
		slog.Error("MoveToMacTrash:: There was an error while creating .trash dir", "error", err, "trashDir", trashDir)
		return err
	}
	slog.Info("MoveToMacTrash:: successfully created the .trash directory", "trashDir", trashDir)
	dest := uniqueTrashDest(trashDir, filepath.Base(path))
	slog.Info("MoveToMacTrash:: Dest", "dest", dest)

	return os.Rename(path, dest)
}

func moveToFreedesktopTrash(path string) error {
	slog.Info("MoveToFreeDesktopTrash:: Path", "path", path)
	home, err := os.UserHomeDir()
	if err != nil {
		slog.Info("MoveToFreeDesktopTrash:: There was an error while getting the user's homedir", "error", err, "path", path)
		return err
	}
	slog.Info("MoveToFreeDesktopTrash:: was able to get the user's homedir", "home", home)

	filesDir := filepath.Join(home, ".local", "share", "Trash", "files")
	slog.Info("MoveToFreeDesktopTrash:: FilesDir", "filesDir", filesDir)

	infoDir := filepath.Join(home, ".local", "share", "Trash", "info")
	slog.Info("MoveToFreeDesktopTrash:: infoDir", "infoDir", infoDir)

	if err := os.MkdirAll(filesDir, 0700); err != nil {
		slog.Error("MoveToFreeDesktopTrash:: Mkdir filesDir failed. ", "filesDir", filesDir)
		return err
	}
	if err := os.MkdirAll(infoDir, 0700); err != nil {
		slog.Error("MoveToFreeDesktopTrash:: Mkdir infoDir failed. ", "infoDir", infoDir)
		return err
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		slog.Error("MoveToFreeDesktopTrash:: AbsolutePath. ", "absPath", absPath)
		return err
	}
	slog.Info("MoveToFreeDesktopTrash:: absPath succedded ", "absPath", absPath)

	dest := uniqueTrashDest(filesDir, filepath.Base(path))
	slog.Info("MoveToFreeDesktopTrash:: Dest", "dest", dest)

	destBase := filepath.Base(dest)
	slog.Info("MoveToFreeDesktopTrash:: DestBase", "destbase", destBase)

	// Write .trashinfo before renaming so the file is never without metadata.
	infoPath := filepath.Join(infoDir, destBase+".trashinfo")
	slog.Info("MoveToFreeDesktopTrash:: InfoPath", "infoPath", infoPath)

	infoContent := fmt.Sprintf(
		"[Trash Info]\nPath=%s\nDeletionDate=%s\n",
		absPath,
		time.Now().Format("2006-01-02T15:04:05"),
	)
	if err := os.WriteFile(infoPath, []byte(infoContent), 0600); err != nil {
		slog.Error("MoveToFreeDesktopTrash:: OS writing file", "err", err)
		return err
	}
	slog.Info("MoveToFreeDesktopTrash:: wrote file successfully")
	if err := os.Rename(path, dest); err != nil {
		slog.Error("MoveToFreeDesktopTrash:: Rename failed", "error", err)
		_ = os.Remove(infoPath) // clean up orphaned .trashinfo on failure
		return err
	}
	slog.Info("MoveToFreeDesktopTrash:: everything worked out well, returning nil")
	return nil
}

// uniqueTrashDest returns a destination path inside dir for a file named base,
// appending a counter suffix if a file with that name already exists.
func uniqueTrashDest(dir, base string) string {
	slog.Info("UniqueTrashDest:: going to work in generating the unique trash dest", "dir", dir, "base", base)
	dest := filepath.Join(dir, base)
	slog.Info("UniqueTrashDest:: Dest path", "dest", dest)
	ext := filepath.Ext(base)
	slog.Info("UniqueTrashDest:: Extension", "ext", ext)

	stem := base[:len(base)-len(ext)]
	for i := 1; ; i++ {
		if _, err := os.Lstat(dest); os.IsNotExist(err) {
			slog.Error("UniqueTrashDest:: There was an error while running the LSTAT command", "error", err, "stem", stem)
			return dest
		}
		dest = filepath.Join(dir, fmt.Sprintf("%s_%d%s", stem, i, ext))
		slog.Info("UniqueTrashDest:: destination location ", "dest", dest)
	}
}
