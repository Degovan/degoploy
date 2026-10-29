---
title: Deploy script
updated: 2026-10-29
---

# Deploy script

The deploy script is what degoploy runs on every deployment, for example `composer install`, `npm run build`, database migrations and a service restart. It runs with **bash** in the project's working directory, after the repository is updated.

Everyone can read the script. Only **admins** can change it. Find it on a project's **Script** tab.

## Two places the script can live

| Source | How you change it | Good for |
|---|---|---|
| **Managed by degoploy** | Edit it on the **Script** tab; every save becomes a revision | Teams that want to change deploys without a commit |
| **From the repository** | Edit and commit a file in the repository, such as `.degoploy/deploy.sh`; degoploy reads it after each pull | Teams that want the script reviewed and versioned with the code |

You pick the source when creating the project and can switch it with **Change source**, which opens [project settings](project-settings.md).

## Edit a managed script

1. Open the **Script** tab. New projects start with a commented template.
2. Edit the script. **Unsaved changes** shows while you have edits.
3. Click **Save revision**, or press **Ctrl+S** / **⌘S**. The new revision is used from the next deployment on.
4. Click **Discard** to drop unsaved edits.

## Revisions

The **Revisions** panel lists every saved version, with who saved it and when.

- Click a revision to **preview** it; the editor turns read-only.
- Click **Restore** to load that revision into the editor, then **Save revision** to make it current. It is saved as a new revision, so no version is ever lost.

Every deployment records the revision it ran, so you can always tell which script a deploy used.

## A script from the repository

With **From the repository**, the **Script** tab shows the file read-only. Before the first pull the file does not exist yet, and the tab says so.

## Variables available to the script

degoploy sets these on every run. Click a name in the app to copy it.

| Variable | Value |
|---|---|
| `DEPLOY_ID` | Deployment number |
| `DEPLOY_PROJECT` | Project slug |
| `DEPLOY_PROJECT_NAME` | Project name |
| `DEPLOY_RULE` | Name of the matched trigger rule |
| `DEPLOY_TRIGGER` | `webhook`, `manual` or `redeploy` |
| `DEPLOY_REF` | Full git ref, such as `refs/heads/main` |
| `DEPLOY_BRANCH` | Branch name, empty for tags |
| `DEPLOY_TAG` | Tag name, empty for branches |
| `DEPLOY_COMMIT` | Full commit SHA |
| `DEPLOY_AUTHOR` | Commit author |
| `DEPLOY_WORKDIR` | Working directory |
| `GIT_SSH_COMMAND` | Makes `git` in the script use the project's deploy key |

Your project's [variables](variables-and-secrets.md) and the rule's **Extra environment** are available too.

```bash
set -euo pipefail
echo "Deploy #$DEPLOY_ID ($DEPLOY_BRANCH @ ${DEPLOY_COMMIT:0:7})"
composer install --no-dev --optimize-autoloader
php artisan migrate --force
sudo systemctl reload php-fpm
```

## Good to know

> [!TIP]
> Start the script with `set -euo pipefail` so it stops at the first failing command.

- A deployment **fails** when the script exits with a code other than 0.
- The script is stopped when it runs longer than the project's **Timeout**, set in [project settings](project-settings.md).
- The script runs as the degoploy system user. Extra rights, such as `sudo` for restarting a service, are set up on the server; see [Installation](../INSTALL.md).
