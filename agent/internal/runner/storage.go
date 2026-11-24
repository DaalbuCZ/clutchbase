package runner

import (
	"math"
	"syscall"

	"clutchbase/internal/protocol"
)

func storageAt(path string) *protocol.MachineStorage {
	var fs syscall.Statfs_t
	if syscall.Statfs(path, &fs) != nil || fs.Bsize <= 0 || fs.Blocks == 0 || fs.Bavail > fs.Blocks || fs.Blocks > uint64(math.MaxInt64)/uint64(fs.Bsize) {
		return nil
	}
	return &protocol.MachineStorage{TotalBytes: int64(fs.Blocks * uint64(fs.Bsize)), AvailableBytes: int64(fs.Bavail * uint64(fs.Bsize))}
}
