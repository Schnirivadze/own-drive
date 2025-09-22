DELETE FROM auth_tokens WHERE expires_at < CURRENT_TIMESTAMP;
DELETE FROM invite_tokens WHERE expires_at < CURRENT_TIMESTAMP;