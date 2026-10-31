---
title: Releases and rollback
updated: 2026-10-31
---

# Releases and rollback

With releases on, every deployment builds in a folder of its own and the site switches to it only when the deploy script succeeds. Visitors never see a half-built release, a failed build changes nothing, and an earlier release is live again in a second.

Without releases, degoploy updates one checkout in place: while `composer install` or `npm run build` runs, the site serves a mix of old and new files, and a failed build leaves it that way.

## How it works

The working directory holds the releases, not the code itself:

```text
/srv/degoploy/shop/
├── current → releases/42      the live release; point the web server here
├── releases/
│   ├── 42/                    deployment #42, live
│   └── 40/                    deployment #40, kept for rollback
├── shared/                    .env, storage/ and uploads, linked into every release
└── repo/                      fetched commits, shared by the releases
```

```mermaid
flowchart LR
    U[Check out<br/>releases/42] --> L[Link shared files] --> S[Run the script<br/>in releases/42]
    S -- success --> C[Switch current<br/>to 42] --> A[After-activate<br/>commands] --> H{Health check}
    S -- failure --> R[Remove releases/42<br/>current stays]
    H -- answers --> OK[Live]
    H -- no answer --> B[Switch back to 40<br/>remove releases/42]
    class OK ok
    class R,B bad
```

1. degoploy checks out the pushed commit into `releases/<deployment number>`.
2. Every **shared path** in the release becomes a link into `shared/`.
3. The deploy script runs **in the release**, not in `current`.
4. When the script succeeds, `current` switches to the new release in one step.
5. The **after-activate commands** run.
6. With a **health check** URL set, degoploy requests it until the app answers. If it does not, `current` switches back to the release that was live and the new one is removed.
7. Releases beyond the number to keep are removed.

If the script fails, times out or is canceled, the release is removed and `current` keeps pointing at the release that was live.

## Turn releases on

> [!IMPORTANT]
> Releases need an empty working directory: the folder of a project that deployed in place holds its checkout. Pick a new folder, or leave Working Directory empty on a new project.

1. In the project's **Settings**, under **Releases**, switch it on. For a new project, turn on **Deploy in releases** in the form.
2. Fill in **Shared paths** and the **After-activate commands** (below) and click **Save**.
3. Deploy once, then point the web server at `current`:

```nginx
root /srv/degoploy/shop/current/public;
```

**Update repository before running** stays on with releases: each release is a fresh checkout.

## Shared paths

Files that must outlive a release: the `.env`, uploaded files, logs and sessions. List them one per line, relative to the repository. End a folder with `/`.

```text
.env
storage/
public/uploads/
```

For each path:

- The first release that has it **moves** its copy into `shared/`. A repository with a `storage/` folder seeds `shared/storage/` that way.
- When no release has it, degoploy creates it empty: a folder for a path ending in `/`, a file otherwise. Fill in an empty `shared/.env` on the server.
- Every release then gets a link to `shared/` in its place, replacing what the commit had.

## After-activate commands

Commands that run with bash in `current` after every switch, rollbacks included. Use them for what must happen once the new code is live:

```bash
sudo systemctl reload php8.3-fpm
```

```bash
pm2 reload ecosystem.config.js
```

They get the same variables as the deploy script. If they fail, the deployment is marked *Failed*, but the release stays live: `current` has already switched.

> [!TIP]
> Reload PHP-FPM after a switch: OPcache keeps serving the old release's files otherwise.

## Health check

A deploy script that succeeds can still leave a broken site: a missing `.env` value, a class that does not load, a PHP-FPM that serves the old code. Under **Health Check** in the project's **Settings**, give degoploy a URL of the app to request after each switch:

```text
https://shop.example.com/up
```

- degoploy requests it with `GET` every 2 seconds, after the after-activate commands, until it answers **2xx or 3xx**. Redirects are not followed: a redirect already means the app answers.
- **Wait up to** (60 seconds by default, 5 to 600) is how long it keeps trying. Each request may take up to 10 seconds. It counts toward the deployment's timeout and must be shorter than it.
- If the app never answers, the deployment is marked *Failed*, `current` switches back to the release that was live, the **after-activate commands** run there, and the new release is removed. The error says so, for example *health check failed: … still gave 500 Internal Server Error after 60s; rolled back to release 40*.
- The log shows each different answer once, under the script's stage.

Laravel answers on `/up` out of the box (Laravel 11 and later); any page that needs the database and the cache works as well.

> [!NOTE]
> The request comes from the degoploy server. A URL that only resolves inside a private network, or a certificate the server does not trust, fails the check.

When there is nothing to switch back to, the failed release stays live and only the deployment fails: on the first release, or when a [rollback](#roll-back) itself fails its check. Without releases, a failed check also only marks the deployment *Failed*.

## Roll back

Under **Releases** on the project page, every release still on the server is listed, the live one marked **Live**.

1. Click **Roll back** on an older release (or **Switch** on a newer one).
2. Confirm.

degoploy queues a deployment with the trigger *Rollback*. It runs no script: it switches `current` and runs the after-activate commands, so it takes a second. It appears in the deployment history like any other, and **Redeploy** on it switches to the same release again.

> [!WARNING]
> A rollback switches the code, not the data. Database migrations the newer release ran stay applied; roll them back yourself if the older code cannot work with them.

## How many releases are kept

**Releases to keep** counts the live one, 5 by default. After each switch, older releases beyond that are removed. A release the web server wrote files into may not be removable; the log says so.

## Variables for releases

Besides the [usual variables](deploy-script.md#variables-available-to-the-script), scripts and after-activate commands get:

| Variable | Value |
|---|---|
| `DEPLOY_RELEASE` | The release number: the deployment that built it |
| `DEPLOY_RELEASE_DIR` | Its folder, `releases/<number>` |
| `DEPLOY_SHARED_DIR` | The `shared/` folder |
| `DEPLOY_CURRENT_DIR` | The `current` link |
| `DEPLOY_WORKDIR` | Where the script runs: the release being built, or `current` for the after-activate commands |
