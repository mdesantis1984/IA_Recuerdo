//go:build !postgres

package store

import (
	"database/sql"
	"fmt"
	"log"

	_ "modernc.org/sqlite"
)

func openDB(cfg Config) (*sql.DB, error) {
	path := cfg.DSN
	if path == "" {
		path = "./ia-recuerdo.db"
	}
	dsn := fmt.Sprintf("file:%s?_journal=WAL&_timeout=5000&_fk=true", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite open: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite ping: %w", err)
	}
	log.Printf("[store] SQLite: %s", path)
	return db, nil
}
