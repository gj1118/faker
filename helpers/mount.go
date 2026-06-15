package helpers

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	diskfs "github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/disk"
	"github.com/diskfs/go-diskfs/filesystem"
	"github.com/diskfs/go-diskfs/filesystem/iso9660"
)

func createISO(srcDir, isoPath string) error {
	fmt.Printf("📦 Scanning folder: %s\n", srcDir)

	dirSize, err := calcDirSize(srcDir)
	if err != nil {
		return fmt.Errorf("calc dir size: %w", err)
	} else {
		fmt.Println("Dir size was calculated")
	}

	// Add 20% overhead for ISO metadata, then align to 2048-byte sector boundary.
	// Mount-DiskImage on Windows requires the file length to be an exact multiple
	// of the ISO 2048-byte sector size.
	const isoSectorSize = int64(2048)
	raw := int64(float64(dirSize)*1.2) + 5*1024*1024 // +5 MB minimum
	isoSize := ((raw + isoSectorSize - 1) / isoSectorSize) * isoSectorSize

	fmt.Printf("📁 Folder size: %d bytes | ISO size: %d bytes\n", dirSize, isoSize)

	d, err := diskfs.Create(isoPath, isoSize, diskfs.SectorSizeDefault)
	if err != nil {
		return fmt.Errorf("create disk: %w", err)
	}
	defer d.Close()
	d.LogicalBlocksize = 2048

	// Pass srcDir as WorkDir so Finalize reads the source files directly,
	// avoiding mixed path-separator issues on Windows from the manual copy step.
	fspec := disk.FilesystemSpec{
		Partition:   0,
		FSType:      filesystem.TypeISO9660,
		VolumeLabel: "MYVOLUME",
		WorkDir:     srcDir,
	}

	fs, err := d.CreateFilesystem(fspec)
	if err != nil {
		return fmt.Errorf("create filesystem: %w", err)
	}

	iso, ok := fs.(*iso9660.FileSystem)
	if !ok {
		return fmt.Errorf("not an ISO9660 filesystem")
	}

	fmt.Println("✅ Finalizing ISO...")
	return iso.Finalize(iso9660.FinalizeOptions{
		VolumeIdentifier: "MYVOLUME",
	})
}

func mountISO(isoPath string) (string, error) {
	switch runtime.GOOS {

	case "linux":
		mountPoint := "/mnt/iso_" + randomSuffix()

		if err := os.MkdirAll(mountPoint, 0755); err != nil {
			return "", fmt.Errorf("create mount point: %w", err)
		}

		cmd := exec.Command("mount", "-o", "loop,ro", isoPath, mountPoint)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("mount failed: %w", err)
		}
		return mountPoint, nil

	case "darwin":
		out, err := exec.Command(
			"hdiutil", "attach", isoPath,
			"-nobrowse",
			"-noverify",
			"-readonly",
		).Output()
		if err != nil {
			return "", fmt.Errorf("hdiutil attach failed: %w", err)
		}

		mountPoint := parseMacMountPoint(string(out))
		if mountPoint == "" {
			return "", fmt.Errorf("could not parse mount point from hdiutil output:\n%s", out)
		}
		return mountPoint, nil

	case "windows":
		// Step 1: mount the image
		mountCmd := fmt.Sprintf(`Mount-DiskImage -ImagePath '%s'`, isoPath)
		if out, err := exec.Command("powershell", "-Command", mountCmd).CombinedOutput(); err != nil {
			return "", fmt.Errorf("Mount-DiskImage failed: %w\n%s", err, out)
		}

		// Step 2: query the drive letter via Get-DiskImage + Get-Volume.
		// Use a small retry loop because the volume may take a moment to be
		// assigned a letter after the image is attached.
		queryCmd := fmt.Sprintf(
			`(Get-DiskImage -ImagePath '%s' | Get-Volume).DriveLetter`,
			isoPath,
		)
		var driveLetter string
		for range 5 {
			out, err := exec.Command("powershell", "-Command", queryCmd).Output()
			if err == nil {
				driveLetter = strings.TrimSpace(string(out))
			}
			if driveLetter != "" {
				break
			}
			// brief pause then retry
			exec.Command("powershell", "-Command", "Start-Sleep -Milliseconds 500").Run()
		}
		if driveLetter == "" {
			return "", fmt.Errorf("could not detect drive letter after mounting")
		}
		return driveLetter + `:\`, nil

	default:
		return "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

//local helpers
func calcDirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}


func parseMacMountPoint(output string) string {
	for line := range strings.SplitSeq(output, "\n") {
		fields := strings.FieldsSeq(line)
		// The mount point is the field starting with /Volumes/
		for f := range fields {
			if strings.HasPrefix(f, "/Volumes/") {
				return f
			}
		}
	}
	return ""
}

func randomSuffix() string {
	b := make([]byte, 4)
	f, _ := os.Open("/dev/urandom")
	defer f.Close()
	f.Read(b)
	return fmt.Sprintf("%x", b)
}
