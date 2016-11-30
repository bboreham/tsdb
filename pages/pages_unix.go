// +build !windows,!plan9,!solaris

package pages

import (
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"
)

// flock acquires an advisory lock on a file descriptor.
func flock(pb *DB, mode os.FileMode, exclusive bool, timeout time.Duration) error {
	var t time.Time
	for {
		// If we're beyond our timeout then return an error.
		// This can only occur after we've attempted a flock once.
		if t.IsZero() {
			t = time.Now()
		} else if timeout > 0 && time.Since(t) > timeout {
			return ErrTimeout
		}
		flag := syscall.LOCK_SH
		if exclusive {
			flag = syscall.LOCK_EX
		}

		// Otherwise attempt to obtain an exclusive lock.
		err := syscall.Flock(int(pb.file.Fd()), flag|syscall.LOCK_NB)
		if err == nil {
			return nil
		} else if err != syscall.EWOULDBLOCK {
			return err
		}

		// Wait for a bit and try again.
		time.Sleep(50 * time.Millisecond)
	}
}

// funlock releases an advisory lock on a file descriptor.
func funlock(pb *DB) error {
	return syscall.Flock(int(pb.file.Fd()), syscall.LOCK_UN)
}

// mmap memory maps a PageBuf's data file.
func mmap(pb *DB, sz int) error {
	// Map the data file to memory.
	b, err := syscall.Mmap(int(pb.file.Fd()), 0, sz, syscall.PROT_READ, syscall.MAP_SHARED|pb.MmapFlags)
	if err != nil {
		return err
	}

	// Advise the kernel that the mmap is accessed randomly.
	if err := madvise(b, syscall.MADV_RANDOM); err != nil {
		return fmt.Errorf("madvise: %s", err)
	}

	// Save the original byte slice and convert to a byte array pointer.
	pb.dataref = b
	pb.data = (*[maxMapSize]byte)(unsafe.Pointer(&b[0]))
	pb.datasz = sz
	return nil
}

// munmap unmaps a PageBuf's data file from memory.
func munmap(pb *DB) error {
	// Ignore the unmap if we have no mapped data.
	if pb.dataref == nil {
		return nil
	}

	// Unmap using the original byte slice.
	err := syscall.Munmap(pb.dataref)
	pb.dataref = nil
	pb.data = nil
	pb.datasz = 0
	return err
}

// NOTE: This function is copied from stdlib because it is not available on darwin.
func madvise(b []byte, advice int) (err error) {
	_, _, e1 := syscall.Syscall(syscall.SYS_MADVISE, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), uintptr(advice))
	if e1 != 0 {
		err = e1
	}
	return
}