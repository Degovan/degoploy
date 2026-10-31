---
title: Users & invites
admin: true
updated: 2026-10-31
---

# Users and invites

The **Users** page decides who can sign in to degoploy and with which role. Only **admins** can open it.

## The user list

One table lists everyone:

- **Active users**, with you first, marked *You*.
- **Pending invitations**, marked *Invited*, or *Expired* after 7 days.
- **Disabled users**, dimmed at the bottom.

Each row shows the role, how the person signs in (*Password* or the SSO name), their status and their last sign-in. The header counts active, invited and disabled accounts.

- Press `/` to search by name, username or email.
- Use **All / Admin / Deployer / Viewer** to show one role.

## Invite people

Invitations are sent by **email**, so [Email (SMTP)](email.md) must be set up and pass a test first. Until then, the invite dialog shows what is missing, with a **Set up SMTP** button.

1. Click **Invite user**.
2. Type an email address and press **Enter**. Add as many as you like, or paste a whole list. Invalid addresses turn red.
3. Choose a role: **Admin**, **Deployer** or **Viewer**. Everyone in the invite gets the same role.
4. Click **Send invite**, or press **Ctrl+Enter** / **⌘Enter**.

The result shows, for each person, whether the email was **Emailed** or **Not sent** and why. Each invitation link is listed too, so you can send it another way if an email fails. Links are shown only this once.

> [!IMPORTANT]
> An invitation works **once** and expires after **7 days**.

- An address that already has an account, or a pending invitation, cannot be invited again.
- Invitations create **password** accounts, so they are unavailable while password sign-in is off. People in your SSO do not need an invitation; they sign in with SSO directly.

### Resend or revoke an invitation

On an invitation's row:

- **Resend** (paper plane): emails a **new** link. The old link stops working, and the 7 days start over.
- **Revoke** (×): the link stops working at once. You can invite the person again later.

## Edit a user

Click the pencil on a user's row:

- **Display name** and **Email**.
- **Role**: pick one of the three role cards.
- **Project access**: every project, or only some (below). Not shown for admins.
- **Disable sign-in**: signs them out everywhere and blocks sign-in until you turn it back on. Their data stays.
- **Delete user**: removes the account. Audit entries and deployments they started are kept.

> [!WARNING]
> There must always be **at least one active admin**. The last admin cannot be demoted, disabled or deleted. You also cannot change your own role, disable yourself or delete your own account.

For SSO users with *Sync role on every sign-in* turned on, a role you set here may be overwritten by their SSO groups the next time they sign in. See [role mapping](single-sign-on.md#map-groups-to-roles).

## Limit a user to some projects

For a Deployer or Viewer, **Project access** decides which projects they see:

- **All projects** (the default): their role applies everywhere, including projects added later.
- **Selected projects**: tick the projects they may see and pick **Deployer** or **Viewer** on each. They see nothing else.

```text
budi  Deployer · 2 projects
  ☑ Company Blog   Deployer
  ☐ Internal API
  ☑ Shop           Viewer
```

The user list shows how many projects a limited user has under their role. Changes apply at once and are recorded in the [audit log](audit-log.md). Promoting a user to Admin clears their selection, since admins see every project. See [Selected projects](roles.md#selected-projects) for what a limited user can do.

## Reset a user's password

Only for users who sign in with a password; SSO users manage their password in the SSO.

1. Click the key on the user's row.
2. Type a new password or click **Generate**. The meter shows how strong it is, and the copy button helps you share it.
3. Keep **Require a new password on next sign-in** checked so the user picks their own password after signing in.
4. Click **Set password**.

The dialog shows how many sessions the user has open; all of them end right away. Share the new password through a safe channel. The user's row shows *Must set a new password* until they change it.
