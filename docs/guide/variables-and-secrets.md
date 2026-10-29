---
title: Variables & secrets
updated: 2026-10-29
---

# Variables and secrets

Each project can have variables that every deploy script receives, such as `APP_URL`. Sensitive values, like a database password or an API key, can be stored as **secrets**.

Find them on a project's **Environment** tab.

- Everyone can see variable **names**.
- Only **admins** can see plain values and add, change or delete variables. Other roles see plain values masked (`••••••`).
- Nobody, admins included, can read a **secret** after it is saved.

## Add a variable

1. Click **Add Variable**.
2. Enter a **Name**: letters, digits and underscores, not starting with a digit, such as `DB_PASSWORD`. Names starting with `DEPLOY_` are reserved for degoploy.
3. Enter the **Value**.
4. Turn on **Secret** if the value is sensitive.
5. Click **Create Variable**.

The variable is used from the next deployment on.

## Change or delete a variable

- Click the pencil to change the name or value, then **Save Changes**.
- A secret's value is never shown again. Leave **Value** empty to keep it, or type a new one to replace it.
- You can turn a plain variable into a secret at any time. Turning a secret back into a plain variable needs the value typed again.
- The **⋯** menu has **Copy name**, **Copy reference** (`${NAME}`, for use in the script) and **Delete**.

## How secrets are protected

- [x] Stored **encrypted** on the server.
- [x] **Write-only**: once saved, a secret can be replaced but never read.
- [x] **Masked in logs**: if the script prints a secret, the log shows `***`.

## Which value wins

When the same name is set in several places, the strongest wins:

1. degoploy's own `DEPLOY_*` variables always win.
2. Then the matched trigger rule's **Extra environment**.
3. Then the project's variables on the **Environment** tab.

For example, a *Staging* rule can override the project's `APP_URL` only for pushes to the staging branch.

## Good to know

> [!WARNING]
> A deleted secret cannot be recovered.

- Deleting a variable takes effect from the next deployment; scripts that read it get an empty value.
- The first lines of every deployment log list the **names** of the variables used, never their values.
