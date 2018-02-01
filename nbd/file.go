package nbd

import (
	"context"
	"os"
	"strconv"
	"syscall"

	"github.com/pkg/errors"
)

// FileBackend implements Backend
type FileBackend struct {
	file *os.File
	size uint64
}

// WriteAt implements Backend.WriteAt
func (fb *FileBackend) WriteAt(ctx context.Context, b []byte, offset int64) (int64, error) {
	n, err := fb.file.WriteAt(b, offset)
	return int64(n), err
}

// WriteZeroesAt implements Backend.WriteZeroesAt
func (fb *FileBackend) WriteZeroesAt(ctx context.Context, offset, length int64) (int64, error) {
	b := make([]byte, length)
	n, err := fb.file.WriteAt(b, offset)
	return int64(n), err
}

// ReadAt implements Backend.ReadAt
func (fb *FileBackend) ReadAt(ctx context.Context, offset, length int64) ([]byte, error) {
	bytes := make([]byte, length)
	_, err := fb.file.ReadAt(bytes, offset)
	return bytes, err
}

// TrimAt implements Backend.TrimAt
func (fb *FileBackend) TrimAt(ctx context.Context, offset, length int64) (int64, error) {
	return length, nil
}

// Flush implements Backend.Flush
func (fb *FileBackend) Flush(ctx context.Context) error {
	return fb.file.Sync()
}

// Close implements Backend.Close
func (fb *FileBackend) Close(ctx context.Context) error {
	return fb.file.Close()
}

// Geometry implements Backend.Geometry
func (fb *FileBackend) Geometry(ctx context.Context) (*Geometry, error) {
	return &Geometry{
		Size:               fb.size,
		MinimumBlockSize:   1,
		MaximumBlockSize:   128 * 1024 * 1024,
		PreferredBlockSize: 32 * 1024,
	}, nil
}

// HasFua implements Backend.HasFua
func (fb *FileBackend) HasFua(ctx context.Context) bool {
	return true
}

// HasFlush implements Backend.HasFlush
func (fb *FileBackend) HasFlush(ctx context.Context) bool {
	return true
}

// NewFileBackend generates a new file backend
func NewFileBackend(ctx context.Context, ec *ExportConfig) (Backend, error) {
	perms := os.O_RDWR
	if ec.ReadOnly {
		perms = os.O_RDONLY
	}

	if ec.DriverParameters == nil {
		return nil, errors.New("required DriverParameters is nil")
	}

	if s, _ := strconv.ParseBool(ec.DriverParameters["sync"]); s {
		perms |= os.O_SYNC
	}

	path := ec.DriverParameters["path"]
	file, err := os.OpenFile(path, perms, 0666)
	if err != nil {
		if !os.IsNotExist(err) || ec.ReadOnly {
			return nil, err
		}
		file, err = os.Create(path)
		if err != nil {
			return nil, err
		}
	}
	size, err := FreeSpace(path)
	if err != nil {
		return nil, err
	}

	return &FileBackend{
		file: file,
		size: size,
	}, nil
}

// FreeSpace return the space available on a disk in bytes
func FreeSpace(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}

	// Available blocks * size per block = available space in bytes
	return uint64(stat.Bavail * uint64(stat.Bsize)), nil
}

// Register our backend
func init() {
	RegisterBackend("file", NewFileBackend)
}
