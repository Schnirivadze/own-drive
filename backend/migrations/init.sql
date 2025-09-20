--init.sql

PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS users (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    username  TEXT UNIQUE NOT NULL,
    password  TEXT NOT NULL,
    quota_bytes INTEGER NOT NULL DEFAULT 0,
    used_bytes  INTEGER NOT NULL DEFAULT 0,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS files (
    id        TEXT PRIMARY KEY,         
    owner_id  INTEGER REFERENCES users(id) ON DELETE SET NULL,
    virtual_path TEXT NOT NULL,       
    original_name TEXT NOT NULL,
    size_bytes    INTEGER NOT NULL,
    mime_type     TEXT,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS tokens
(
	token TEXT PRIMARY KEY, 
	user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
	created_at DATETIME CURRENT_TIMESTAMP, 
	expires_at DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_files_owner ON files(owner_id);
CREATE INDEX IF NOT EXISTS idx_tokens_expiry ON tokens(expires_at);