---
title: Roles
updated: 2026-10-29
---

# Roles and permissions

degoploy has three built-in roles. Every user has exactly one, and it applies to every project.

Admins can compare the roles and see their members under **Roles** in the header. Roles are given on the [Users page](users-and-invites.md) or automatically through [SSO role mapping](single-sign-on.md#map-groups-to-roles).

## Compare roles

| Can | Admin | Deployer | Viewer |
|---|:-:|:-:|:-:|
| **Projects & deployments** | | | |
| See projects and deployment history | ✓ | ✓ | ✓ |
| Read build logs | ✓ | ✓ | ✓ |
| Deploy manually, cancel and redeploy | ✓ | ✓ | – |
| **Configuration** | | | |
| Create and delete projects | ✓ | – | – |
| Edit trigger rules, deploy scripts and project settings | ✓ | – | – |
| Manage variables and secrets, see the deploy key and webhook | ✓ | – | – |
| **Administration** | | | |
| Manage users, invitations and password resets | ✓ | – | – |
| Configure sign-in methods, SSO and email | ✓ | – | – |
| View the audit log | ✓ | – | – |

## Good to know

- Menus and buttons you cannot use are hidden. A Viewer does not see **Deploy now**, for example, and only admins see a project's **Settings** tab.
- Role changes apply **immediately**, even for people who have degoploy open.
- The built-in roles cannot be changed, and new roles cannot be added.

> [!IMPORTANT]
> There must always be at least **one active admin**.
