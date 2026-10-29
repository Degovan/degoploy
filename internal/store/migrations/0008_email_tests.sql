-- Test emails sent from the Email page. Invites are enabled while the latest
-- successful test was sent with the current SMTP settings (settings_mac is a
-- keyed fingerprint of them, see secrets.Box.MAC).
CREATE TABLE email_tests (
    id            INTEGER PRIMARY KEY,
    recipient     TEXT NOT NULL,
    ok            INTEGER NOT NULL,
    detail        TEXT NOT NULL,
    settings_mac  TEXT NOT NULL,
    sent_by       INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at    DATETIME NOT NULL
);
