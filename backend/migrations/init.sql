--init.sql
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	username TEXT UNIQUE NOT NULL,
	password TEXT NOT NULL,
	role TEXT NOT NULL DEFAULT 'user',
	quota_bytes INTEGER NOT NULL DEFAULT 0,
	used_bytes INTEGER NOT NULL DEFAULT 0,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS auth_tokens (
	token TEXT PRIMARY KEY,
	user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
	created_at DATETIME CURRENT_TIMESTAMP,
	expires_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS invite_tokens (
	token TEXT PRIMARY KEY,
	created_at DATETIME CURRENT_TIMESTAMP,
	expires_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS folders (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	owner_id INTEGER NOT NULL REFERENCES users(id),
	name TEXT NOT NULL,
	parent_id INTEGER REFERENCES folders(id),
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(owner_id, parent_id, name)
);

CREATE TABLE IF NOT EXISTS files (
	uuid TEXT PRIMARY KEY,
	owner_id INTEGER NOT NULL REFERENCES users(id),
	folder_id INTEGER REFERENCES folders(id),
	stored_name TEXT NOT NULL,
	display_name TEXT NOT NULL,
	mime TEXT,
	size_bytes INTEGER NOT NULL,
	size_bytes_on_disk INTEGER,
	sha256 TEXT NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	deleted_at DATETIME NULL
);

CREATE INDEX IF NOT EXISTS idx_files_owner ON files(owner_id);
CREATE INDEX IF NOT EXISTS idx_auth_tokens_expiry ON auth_tokens(expires_at);
CREATE INDEX IF NOT EXISTS idx_invite_tokens_expiry ON invite_tokens(expires_at);
CREATE INDEX IF NOT EXISTS idx_files_folder ON files(folder_id);
CREATE INDEX IF NOT EXISTS idx_folders_parent ON folders(owner_id, parent_id);