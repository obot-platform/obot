---
title: Skill Access Policies
---

## Overview

Skill Access Policies control which users and groups can discover and install which skills. Administrators create policies to grant skill access based on organizational needs—whether that means giving everyone access to all skills, restricting certain skill sources to specific teams, or granting access to individual skills.

Without a policy granting access, regular users cannot see or install any skills. Administrators always have full access regardless of policies.

## How Policies Work

Each policy defines two things:

- **Who** can access the skills (users and groups)
- **Which** skills they can access

When an agent searches for skills on behalf of a user, only skills granted through one or more policies are returned. If no policy grants a user access to any skills, the agent's skill search returns no results.

### Users and Groups

A policy can grant access to:

- **Individual users** — Select specific people by name
- **Groups** — Select authentication provider groups (such as "engineering" or "data-science")
- **Everyone** — Use the "All Users" option to grant access to all authenticated users

Using "All Users" is convenient for making a baseline set of skills universally available, while separate policies can grant additional skills to specific teams.

### Skills

When adding skills to a policy, you can select:

- **Individual skills** — Specific skills by name
- **Entire skill sources** — All skills from a particular source repository, including any skills added to that repository in the future
- **All skills** — Grants access to every skill across all sources, including skills added in the future

Skills are displayed grouped by their source repository, making it easy to find and select related skills.

## Managing Policies

To manage policies, go to **Obot Agent Management > Skill Access Policies**.

### Creating a Policy

1. Click **Create Policy**
2. Enter a descriptive **Name** for the policy
3. Add **Users & Groups** — search for and select the users or groups who should have access
4. Add **Skills** — search for and select individual skills, entire skill sources, or all skills
5. Click **Create**

### Editing a Policy

Click any policy in the list to view and modify its name, users and groups, or skills. Changes take effect immediately.

### Deleting a Policy

Deleting a policy removes skill access for the affected users. If a user loses access to all skills as a result, agents acting on their behalf will no longer be able to search for or install skills until another policy grants them access.

Skills that were already installed before losing access remain available in the agent session.

## Example: Data Science Team

To give your data science team access to data-related skills:

1. Create a policy named "Data Science Skills"
2. Add the "data-science" group as a subject
3. Add the skill source repository that contains your data analysis and visualization skills
4. Save the policy

Members of the data science group can now search for and install any skill from that repository. As new skills are added to the repository, they automatically become available to the team.

## Multiple Policies

Access is additive across policies. If a user matches multiple policies, they get access to the combined set of skills from all matching policies. There is no way to deny access through a policy—policies only grant access.

## Related Topics

- [Skills](../skills) — Manage skill sources and browse available skills
