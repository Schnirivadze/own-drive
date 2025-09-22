DELETE FROM auth_tokens WHERE expires_at < CURRENT_TIMESTAMP;
DELETE FROM auth_tokens WHERE user_id IS NULL;
DELETE FROM invite_tokens WHERE expires_at < CURRENT_TIMESTAMP;