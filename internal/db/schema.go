package db

// Schema contains the SQL schema for WCE system tables
// Based on SECURITY.md specifications
const Schema = `
-- ============================================================================
-- WCE System Tables
-- These tables are automatically created in each new cenv
-- ============================================================================

-- ----------------------------------------------------------------------------
-- User Authentication and Management
-- ----------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS _wce_users (
    user_id TEXT PRIMARY KEY,           -- UUID generated on creation
    username TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,        -- bcrypt hash (cost 12)
    role TEXT NOT NULL DEFAULT 'viewer', -- owner, admin, editor, viewer
    email TEXT,                         -- Optional, for password reset
    created_at INTEGER NOT NULL,        -- Unix timestamp
    invited_by TEXT,                    -- user_id of inviter
    last_login INTEGER,                 -- Unix timestamp
    enabled INTEGER DEFAULT 1,          -- 1 = enabled, 0 = disabled (BOOLEAN)
    FOREIGN KEY (invited_by) REFERENCES _wce_users(user_id)
);

CREATE INDEX IF NOT EXISTS idx_users_username ON _wce_users(username);
CREATE INDEX IF NOT EXISTS idx_users_enabled ON _wce_users(enabled);

-- ----------------------------------------------------------------------------
-- Session Management and Token Revocation
-- ----------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS _wce_sessions (
    session_id TEXT PRIMARY KEY,        -- UUID or JWT jti claim
    user_id TEXT NOT NULL,
    token_hash TEXT NOT NULL,           -- SHA256 of JWT for revocation
    created_at INTEGER NOT NULL,        -- Unix timestamp
    expires_at INTEGER NOT NULL,        -- Unix timestamp
    last_used INTEGER,                  -- Unix timestamp
    ip_address TEXT,
    user_agent TEXT,
    FOREIGN KEY (user_id) REFERENCES _wce_users(user_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON _wce_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_token_hash ON _wce_sessions(token_hash);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON _wce_sessions(expires_at);

-- ----------------------------------------------------------------------------
-- Table-Level Permissions
-- ----------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS _wce_table_permissions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    table_name TEXT NOT NULL,
    user_id TEXT NOT NULL,
    can_read INTEGER DEFAULT 0,         -- 1 = allowed, 0 = denied (BOOLEAN)
    can_write INTEGER DEFAULT 0,        -- 1 = allowed, 0 = denied (BOOLEAN)
    can_delete INTEGER DEFAULT 0,       -- 1 = allowed, 0 = denied (BOOLEAN)
    can_grant INTEGER DEFAULT 0,        -- Can grant permissions to others
    FOREIGN KEY (user_id) REFERENCES _wce_users(user_id) ON DELETE CASCADE,
    UNIQUE(table_name, user_id)
);

CREATE INDEX IF NOT EXISTS idx_table_perms_user ON _wce_table_permissions(user_id);
CREATE INDEX IF NOT EXISTS idx_table_perms_table ON _wce_table_permissions(table_name);

-- ----------------------------------------------------------------------------
-- Row-Level Security Policies
-- ----------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS _wce_row_policies (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    table_name TEXT NOT NULL,
    user_id TEXT,                       -- NULL means applies to all users
    policy_type TEXT NOT NULL,          -- 'read', 'write', 'delete'
    sql_condition TEXT NOT NULL,        -- e.g., "owner_id = $user_id"
    created_at INTEGER NOT NULL,        -- Unix timestamp
    created_by TEXT,                    -- user_id who created the policy
    FOREIGN KEY (user_id) REFERENCES _wce_users(user_id) ON DELETE CASCADE,
    FOREIGN KEY (created_by) REFERENCES _wce_users(user_id)
);

CREATE INDEX IF NOT EXISTS idx_row_policies_table ON _wce_row_policies(table_name);
CREATE INDEX IF NOT EXISTS idx_row_policies_user ON _wce_row_policies(user_id);
CREATE INDEX IF NOT EXISTS idx_row_policies_type ON _wce_row_policies(policy_type);

-- ----------------------------------------------------------------------------
-- Per-Cenv Configuration
-- ----------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS _wce_config (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at INTEGER NOT NULL,        -- Unix timestamp
    updated_by TEXT,                    -- user_id who updated
    FOREIGN KEY (updated_by) REFERENCES _wce_users(user_id)
);

-- ----------------------------------------------------------------------------
-- Audit Logging
-- ----------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS _wce_audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp INTEGER NOT NULL,         -- Unix timestamp
    user_id TEXT NOT NULL,
    username TEXT NOT NULL,             -- Denormalized for convenience
    action TEXT NOT NULL,               -- e.g., 'login', 'create_user', 'grant_permission'
    resource_type TEXT,                 -- e.g., 'user', 'table', 'policy'
    resource_id TEXT,                   -- ID of affected resource
    details TEXT,                       -- JSON with additional context
    ip_address TEXT,
    user_agent TEXT,
    FOREIGN KEY (user_id) REFERENCES _wce_users(user_id)
);

CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON _wce_audit_log(timestamp);
CREATE INDEX IF NOT EXISTS idx_audit_user ON _wce_audit_log(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_action ON _wce_audit_log(action);

-- ----------------------------------------------------------------------------
-- Default Configuration Values
-- ----------------------------------------------------------------------------

INSERT OR IGNORE INTO _wce_config (key, value, updated_at) VALUES
    ('session_timeout_hours', '24', strftime('%s', 'now')),
    ('allow_registration', 'false', strftime('%s', 'now')),
    ('max_users', '10', strftime('%s', 'now'));
`
