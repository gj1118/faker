//go:build windows

package support

import (
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

var (
	modShell32   = syscall.NewLazyDLL("shell32.dll")
	procSHFileOp = modShell32.NewProc("SHFileOperationW")
	shFileOpMu   sync.Mutex
)

// Windows constants for SHFileOperationW.
const (
	foDelete          = uint32(0x0003)
	fofAllowUndo      = uint16(0x0040)
	fofNoConfirmation = uint16(0x0010)
	fofSilent         = uint16(0x0004)
	fofNoErrorUI      = uint16(0x0400)
)

// fAnyOperationsAborted=36, hNameMappings=40, lpszProgressTitle=48.
type shFileOpStructW struct {
	hwnd    uintptr
	wFunc   uint32
	_       uint32 // padding: align pFrom to 8-byte boundary
	pFrom   uintptr
	pTo     uintptr
	fFlags  uint16
	_       uint16 // padding: align fAnyOp to 4-byte boundary
	fAnyOp  int32
	hNameM  uintptr
	lpTitle uintptr
}

func MoveToRecycleBin(path string) error {
	// SHFileOperationW requires a double-null-terminated UTF-16 string.
	from, err := syscall.UTF16FromString(path)
	if err != nil {
		slog.Error("MoveToRecycleBin:: failed to make a syscall.", "error", err, "path", path)
		return err
	}
	slog.Info("MoveToRecycleBin:: syscall succeeded", "path", path)
	from = append(from, 0) // append second null terminator

	op := shFileOpStructW{
		wFunc:  foDelete,
		pFrom:  uintptr(unsafe.Pointer(&from[0])),
		fFlags: fofAllowUndo | fofNoConfirmation | fofSilent | fofNoErrorUI,
	}

	shFileOpMu.Lock()
	ret, _, _ := procSHFileOp.Call(uintptr(unsafe.Pointer(&op)))
	shFileOpMu.Unlock()
	runtime.KeepAlive(from) // prevent GC of from before the syscall completes

	if ret != 0 {
		slog.Error("MoveToRecycleBin:: SHFileOperationW failed", "errorcode", ret)
		return fmt.Errorf("SHFileOperationW failed with code %d", ret)
	}
	slog.Info("MoveToRecycleBin:: MoveToRecycleBin happened successfully and no issues were reported")
	return nil
}
