-- What degoploy answered each webhook delivery (pong, queued, skipped, ignored,
-- rejected). Older rows are classified from the reason they were stored with.
ALTER TABLE webhook_events ADD COLUMN status TEXT NOT NULL DEFAULT '';
UPDATE webhook_events SET status = CASE
    WHEN matched THEN 'queued'
    WHEN reason LIKE '%signature%' OR reason LIKE 'invalid payload:%' THEN 'rejected'
    WHEN reason = 'webhook is configured correctly' THEN 'pong'
    WHEN reason LIKE '% events are not used' THEN 'ignored'
    ELSE 'skipped'
END;

-- Whether the run updated the repository before its script (the project's auto_pull
-- when it started). NULL until a deployment starts, and for runs recorded earlier.
ALTER TABLE deployments ADD COLUMN auto_pull INTEGER;
