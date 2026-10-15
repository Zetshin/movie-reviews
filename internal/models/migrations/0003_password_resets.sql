-- Password reset tokens are stored only as SHA-256 hashes. A leaked database
-- therefore does not contain usable reset links.
ALTER TABLE users ADD COLUMN IF NOT EXISTS auth_version INT UNSIGNED NOT NULL DEFAULT 1;

CREATE TABLE IF NOT EXISTS password_reset_tokens (
    token_hash BINARY(32) PRIMARY KEY,
    user_id    INT NOT NULL,
    expires_at DATETIME(6) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    INDEX password_reset_user_idx (user_id),
    INDEX password_reset_expiry_idx (expires_at),
    CONSTRAINT password_reset_fk_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
