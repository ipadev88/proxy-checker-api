package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/proxy-checker-api/internal/types"
)

type Storage interface {
	Save(snapshot *types.Snapshot) error
	Load() (*types.Snapshot, error)
	Close() error
}

func NewStorage(storageType string, path string) (Storage, error) {
	switch storageType {
	case "file":
		return NewFileStorage(path)
	case "sqlite":
		return NewSQLiteStorage(path)
	case "redis":
		return NewRedisStorage(path)
	default:
		return nil, fmt.Errorf("unknown storage type: %s", storageType)
	}
}

// FileStorage stores snapshots as JSON files
type FileStorage struct {
	path string
}

func NewFileStorage(path string) (*FileStorage, error) {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create directory: %w", err)
	}

	return &FileStorage{path: path}, nil
}

func (f *FileStorage) Save(snapshot *types.Snapshot) error {
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}

	// Atomic write: write to temp file, then rename
	tempPath := f.path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := os.Rename(tempPath, f.path); err != nil {
		return fmt.Errorf("atomic rename: %w", err)
	}

	return nil
}

func (f *FileStorage) Load() (*types.Snapshot, error) {
	data, err := os.ReadFile(f.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // File doesn't exist yet
		}
		return nil, fmt.Errorf("read file: %w", err)
	}

	var snap types.Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("unmarshal JSON: %w", err)
	}

	return &snap, nil
}

func (f *FileStorage) Close() error {
	return nil
}

