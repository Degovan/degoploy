---
title: Audit log
admin: true
updated: 2026-10-29
---

# Audit log

The audit log records **who did what, and when**: sign-ins, project changes, manual deploys, user changes, sign-in and email settings, and more. Entries cannot be edited or deleted from the app.

Only **admins** can open it, from **Audit log** in the header.

## Reading the log

- Activity is grouped **by day** (Today, Yesterday, then dates), with a count for each day. The newest is at the top.
- Each entry reads as a sentence, such as *admin deployed shop #42* or *admin changed the role of Nadia Putri Viewer → Deployer*. Project and deployment names are links.
- The small badge on the avatar shows the category. The IP address is on the right.
- The same action repeated in a row (viewing secrets several times, for example) folds into one entry marked **+N more since …**. Click it to expand.
- The header counts today's events and how many are sensitive.
- Click **Load older** at the bottom for earlier activity.

## Sensitive activity

Actions that touch access or secrets carry a yellow **Sensitive** badge:

- failed sign-ins;
- viewing a project's secrets, regenerating its deploy key or webhook secret;
- deleting a project or a user;
- resetting a user's password;
- changing sign-in, SSO or email settings.

## Search and filter

- **Search** (press `/`): matches people, projects, targets (such as a variable name or an email) and IP addresses.
- **Category**: All activity, Sign-in, Projects, Trigger rules, Variables, Deployments, Users, Invites, Sign-in methods and Email.
- **Sensitive only**: a switch in the category menu. Clicking the *sensitive* count in the header does the same.
- **Person**: show one user's activity.

Filters combine. The bottom shows *Showing X of Y events* for the current filters; with no results, click **Clear filters**.
