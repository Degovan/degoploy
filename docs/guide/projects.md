---
title: Creating a project
updated: 2026-10-29
---

# Creating a project

A project is one application degoploy deploys: its repository, the folder on the server it runs from, and its deploy script.

Everyone can see projects. Only **admins** can create, change and delete them.

## The project list

**Projects** shows every project as cards or a list. For each one you see:

- The latest deployment: status, number, branch and when.
- **Health**: a weather icon and the share of the **last 10 deployments** that succeeded. Sunny means nearly all passed; a storm means many failed.

The header counts healthy, failing and running projects. Press `/` to search. Below the projects, **Recent deployments** lists the newest runs across all projects.

## Create a project

1. On **Projects**, click **New Project**.
2. Fill in **Configure Project**:

   | Field | What it is |
   |---|---|
   | **Project Name** | The name shown in degoploy |
   | **Slug** | A short name used in URLs and folder names. It cannot change later |
   | **Git Provider** | GitHub or Gitea |
   | **Repository URL** | The SSH address, for example `git@github.com:acme/shop.git` |
   | **Working Directory** | The folder on the server the repository is cloned into and the script runs in, for example `/var/www/shop` |
   | **Branch to Deploy** | Creates the first trigger rule for this branch. Optional |

3. Under **Deploy Script**, pick where the script comes from:
   - **Managed by degoploy**: written and edited in the app, with revision history. Choose whether to **Update repository before running**.
   - **From the repository**: read from a file in the repository, such as `.degoploy/deploy.sh`, after each pull.
4. Under **Advanced**, set the **Timeout** for one run in seconds, and whether the project is **Enabled** for webhooks.
5. Click **Create Project**. degoploy creates a **deploy key** and a **webhook secret** for the project, plus the first trigger rule if you entered a branch.

## Connect the repository

The project page shows two cards to set up on GitHub or Gitea. Only admins see them.

### Add the deploy key

This lets degoploy read a private repository.

1. On the **Deploy Key** card, copy the public key.
2. In the repository, open **Settings → Deploy keys** and add it. **Read-only** access is enough.
3. Back in degoploy, click **Test connection** to check that degoploy can read the repository.

### Add the webhook

This tells degoploy about every push.

1. On the **Webhook** card, copy the **webhook URL** and the **webhook secret**.
2. In the repository, open **Settings → Webhooks** and add a webhook with:
   - the URL and secret from degoploy,
   - content type `application/json`,
   - **push** events only.
3. Save it. GitHub sends a *ping* right away, and the Webhook card turns green once a delivery arrives.

> [!TIP]
> If the webhook URL shows the wrong address (such as `localhost`), ask whoever runs the server to set `base_url` in the degoploy config. See [Installation](../INSTALL.md).

## The project page

Opening a project shows:

- **Latest Deployment**: the newest run and its stages.
- **Stage View**: recent runs by stage, with average durations.
- **Webhook Deliveries**: incoming webhooks and their outcome. *queued* started a deploy, *ping* was a test, *rejected* had the wrong secret, and *skipped* matched no rule.
- A project summary and, for admins, the deploy key and webhook cards.
- **Deploy now** for a manual deploy (deployers and admins).

The tabs at the top are **Overview**, **Triggers**, **Script**, **Environment** and, for admins, **Settings**.

## Troubleshooting

<details>
<summary>I pushed, but nothing deployed</summary>

Open **Webhook Deliveries** on the project page:

- No delivery at all: the webhook is missing or its URL is wrong.
- *rejected*: the secret in GitHub or Gitea differs from the one in degoploy.
- *skipped*: no trigger rule matched. See [trigger rules](trigger-rules.md).

</details>

<details>
<summary>The deployment fails at "Update repo"</summary>

Usually the deploy key is not added to the repository, or it was regenerated. Click **Test connection** on the Deploy Key card.

</details>
