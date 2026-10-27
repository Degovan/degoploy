-- When the deploy script started (after the repository update), so the UI can
-- show per-stage durations, and a one-line summary of why a deployment failed.
ALTER TABLE deployments ADD COLUMN script_started_at DATETIME;
ALTER TABLE deployments ADD COLUMN error TEXT NOT NULL DEFAULT '';
