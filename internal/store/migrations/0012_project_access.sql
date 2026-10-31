-- Project access: a deployer or viewer with selected_projects sees only the
-- projects listed for them in project_members, each with its own role. Others
-- keep their global role on every project; admins always do.
ALTER TABLE users ADD COLUMN selected_projects INTEGER NOT NULL DEFAULT 0;

-- project_members was created unused; its roles are now deployer and viewer
-- only, since editing a project stays with admins.
DROP TABLE project_members;
CREATE TABLE project_members (
    project_id  INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role        TEXT NOT NULL CHECK (role IN ('deployer', 'viewer')),
    PRIMARY KEY (project_id, user_id)
);
CREATE INDEX project_members_user ON project_members(user_id);
