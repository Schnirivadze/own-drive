package main

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"os"
)

func openDB(path string) (*sql.DB, error) {

	dsn := "file:" + path + "?_busy_timeout=5000&_foreign_keys=1"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

func runSqlFromFile(db *sql.DB, path string) error {
	sqlBytes, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	_, err = db.Exec(string(sqlBytes))
	return err
}
