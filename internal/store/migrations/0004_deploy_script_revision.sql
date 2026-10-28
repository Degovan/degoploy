-- The managed script revision a deployment ran, so the UI can tell which
-- version of deploy.sh produced a log. NULL for repository scripts and for
-- deployments recorded before degoploy kept it.
ALTER TABLE deployments ADD COLUMN script_revision_id INTEGER REFERENCES script_revisions(id) ON DELETE SET NULL;
