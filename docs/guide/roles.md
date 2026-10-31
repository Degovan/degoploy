---
title: Roles
updated: 2026-10-31
---

# Roles and permissions

degoploy has three built-in roles. Every user has exactly one, and it applies to every project, unless the user is limited to [selected projects](#selected-projects).

Admins can compare the roles and see their members under **Roles** in the header. Roles are given on the [Users page](users-and-invites.md) or automatically through [SSO role mapping](single-sign-on.md#map-groups-to-roles).

## Compare roles

| Can | Admin | Deployer | Viewer |
|---|:-:|:-:|:-:|
| **Projects & deployments** | | | |
| See projects and deployment history | ✓ | ✓ | ✓ |
| Read build logs | ✓ | ✓ | ✓ |
| Deploy manually, cancel, redeploy and roll back | ✓ | ✓ | – |
| **Configuration** | | | |
| Create and delete projects | ✓ | – | – |
| Edit trigger rules, deploy scripts and project settings | ✓ | – | – |
| Manage variables and secrets, see the deploy key and webhook | ✓ | – | – |
| **Administration** | | | |
| Manage users, invitations and password resets | ✓ | – | – |
| Configure sign-in methods, SSO and email | ✓ | – | – |
| View the audit log | ✓ | – | – |

## Selected projects

A Deployer or Viewer can be limited to some projects, under **Project access** when you [edit the user](users-and-invites.md#limit-a-user-to-some-projects). They then:

- see only those projects: the others are missing from the project list, the recent deployments and live updates, and opening one says *Project not found*;
- act on each with the role picked for it, **Deployer** or **Viewer**, whatever their global role;
- do not see projects created later until an admin adds them.

Editing a project stays with admins, so a project role is never Admin. Admins always see every project.

## Good to know

- Menus and buttons you cannot use are hidden. A Viewer does not see **Deploy now**, for example, and only admins see a project's **Settings** tab.
- Role changes apply **immediately**, even for people who have degoploy open.
- The built-in roles cannot be changed, and new roles cannot be added.

> [!IMPORTANT]
> There must always be at least **one active admin**.
