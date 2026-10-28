-- Set when an admin resets a password and asks for a new one at next sign-in.
-- While set, the API only allows changing the password (see auth.RequireAuth).
ALTER TABLE users ADD COLUMN must_change_password INTEGER NOT NULL DEFAULT 0;
