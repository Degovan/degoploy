-- Health check: after a deployment (with releases, once current switched and
-- the after-activate commands ran), degoploy requests health_url until it
-- answers 2xx or 3xx, for at most health_timeout seconds. With releases, a
-- failed check switches current back to the release that was live.
ALTER TABLE projects ADD COLUMN health_url TEXT NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN health_timeout INTEGER NOT NULL DEFAULT 60;
