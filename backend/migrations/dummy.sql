INSERT OR IGNORE INTO users (username, password, quota_bytes)
VALUES ('danya', 'joemama', 25000000000);

INSERT OR IGNORE INTO folders (owner_id, name) 
VALUES (1, '~');

INSERT OR IGNORE INTO users (username, password, quota_bytes, role)
VALUES ('andrii', 'ligma', 50000000000, 'admin');

INSERT OR IGNORE INTO folders (owner_id, name) 
VALUES (2, '~');

INSERT OR IGNORE INTO users (username, password, quota_bytes)
VALUES ('max', 'mamka', 25000000000);

INSERT OR IGNORE INTO folders (owner_id, name) 
VALUES (3, '~');