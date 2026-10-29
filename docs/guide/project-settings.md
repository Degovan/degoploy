---
title: Project settings
admin: true
updated: 2026-10-29
---

# Project settings

Change what you set up when creating the project: its name, repository, working directory, script source, Node.js version, timeout, deploy key and webhook, or delete it.

Only **admins** can open a project's **Settings** tab.

## Saving changes

Every card has its own **Save** button, so you can change one thing without touching the rest. **Save** turns on once you change something. The links on the left (General, Repository, Deploy Script, Deploy Key & Webhook, Danger Zone) jump to each part.

If you leave the page with unsaved changes, degoploy asks first.

## General

- **Project Name**: the name shown in degoploy, up to 100 characters.
- **Project Slug**: shown for reference; it cannot change after the project is created.
- **Accept Webhooks**: turn off to pause automatic deploys. Pushes are still logged under Webhook Deliveries but start nothing. **Deploy now** keeps working.

## Repository

- **Git Repository**: the provider and repository URL. Use the SSH URL for a private repository; degoploy signs in with the project's deploy key. Changing the URL keeps the existing working directory.
- **Working Directory**: the folder on the server the repository is cloned into (on the first deploy) and the script runs in.

## Deploy script

- Choose **Managed by degoploy** or **From the repository**. See [Deploy script](deploy-script.md).
- For a script from the repository, enter its path.
- Choose whether the repository is updated before the script runs.
- **Node.js**: Auto (from `.nvmrc` and similar files), a version such as `22`, or None. See [Node.js versions](nodejs.md).
- **Timeout**: runs longer than this are stopped and marked *Timed out*. Between 10 seconds and 24 hours.

## Deploy key and webhook

The same cards as on the project page:

- **Regenerate** the deploy key: the old key stops working at once, and deploys fail until the new public key is added to the repository.
- **Regenerate secret** for the webhook: webhooks signed with the old secret are rejected until you paste the new one into the repository's webhook settings.
- **Test connection**: checks that degoploy can read the repository.

## Delete a project

1. Under **Danger Zone**, click **Delete**.
2. Type the project's **slug** to confirm.

This removes the project, its deploy key, the managed script and its revisions, variables and deployment history.

> [!CAUTION]
> Deleting a project cannot be undone. Files in the working directory are **left untouched**.
