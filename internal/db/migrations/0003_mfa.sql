-- ISS-019 phase 6: optional TOTP two-factor login. See
-- docs/plans/verified-auth-plan.md.

-- totp_secret is the base32 shared secret ('' = none). While setup is in
-- progress it is set and totp_enabled_at is NULL; MFA is on iff
-- totp_enabled_at is set. totp_last_step is the newest 30-second step a code
-- was accepted for: a code is only good once, even inside its window.
ALTER TABLE users ADD COLUMN totp_secret     TEXT        NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN totp_enabled_at TIMESTAMPTZ;
ALTER TABLE users ADD COLUMN totp_last_step  BIGINT      NOT NULL DEFAULT 0;

-- One-time recovery codes, stored hashed; used_at marks a spent one.
CREATE TABLE recovery_codes (
    user_id   BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash TEXT        NOT NULL,
    used_at   TIMESTAMPTZ,
    PRIMARY KEY (user_id, code_hash)
);

-- "Password OK, waiting for the second factor": a short-lived, hashed token
-- in a cookie, with its own try count. next is where login returns to.
CREATE TABLE pending_logins (
    token_hash TEXT PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    next       TEXT        NOT NULL DEFAULT '/',
    attempts   INTEGER     NOT NULL DEFAULT 0,
    expires_at TIMESTAMPTZ NOT NULL
);
