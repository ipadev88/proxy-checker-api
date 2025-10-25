package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/proxy-checker-api/internal/snapshot"
)

type SQLiteStorage struct {
	db *sql.DB
}

func NewSQLiteStorage(path string) (*SQLiteStorage, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Create table
	schema := `
	CREATE TABLE IF NOT EXISTS snapshots (
		id INTEGER PRIMARY KEY,
		data TEXT NOT NULL,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	`
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("create table: %w", err)
	}

	return &SQLiteStorage{db: db}, nil
}

func (s *SQLiteStorage) Save(snapshot *snapshot.Snapshot) error {
	data, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}

	// Keep only the latest snapshot
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM snapshots"); err != nil {
		return fmt.Errorf("delete old snapshots: %w", err)
	}

	if _, err := tx.Exec("INSERT INTO snapshots (data, updated_at) VALUES (?, ?)",
		string(data), time.Now()); err != nil {
		return fmt.Errorf("insert snapshot: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

func (s *SQLiteStorage) Load() (*snapshot.Snapshot, error) {
	var data string
	err := s.db.QueryRow("SELECT data FROM snapshots ORDER BY id DESC LIMIT 1").Scan(&data)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query snapshot: %w", err)
	}

	var snap snapshot.Snapshot
	if err := json.Unmarshal([]byte(data), &snap); err != nil {
		return nil, fmt.Errorf("unmarshal JSON: %w", err)
	}

	return &snap, nil
}

func (s *SQLiteStorage) Close() error {
	return s.db.Close()
}

