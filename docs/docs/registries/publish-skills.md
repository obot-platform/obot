---
title: "Skills"
---

## Overview {#skills-overview}

Skills are reusable, structured instructions that agents can discover and install to expand their capabilities. Each skill is a self-contained package stored in a git repository, containing a `SKILL.md` file with a description, metadata, and the instructions themselves. Obot indexes skills from configured sources and makes them available to agents based on access policies.

Administrators manage skill sources and control who can access which skills. Agents search, install, and use skills during conversations.


## What Is a Skill? {#skills-what-is-a-skill}

A skill is a directory in a git repository that contains a `SKILL.md` file. This file uses YAML frontmatter to define the skill's identity—its name, description, license, and compatibility requirements—followed by markdown content with the actual instructions.

Skills can also include supporting files alongside the `SKILL.md`, such as helper scripts or reference data. When an agent installs a skill, the entire directory is downloaded.

Each skill has:

- **Name** — A unique identifier within the repository (e.g., `code-review`)
- **Display Name** — A human-readable name generated from the skill name (e.g., "Code Review")
- **Description** — A summary of what the skill does
- **License** — The skill's license, if specified
- **Compatibility** — Any requirements or constraints (e.g., "Python 3.8+")

Obot follows the Agent Skills standard. See [agentskills.io](https://agentskills.io) for more information about skills.


## Skill Sources {#skills-skill-sources}

Skill sources are git repositories that contain one or more skills. When you add a source, Obot scans the repository for directories containing `SKILL.md` files and indexes each valid skill it finds. Sources are synced automatically every hour and can also be refreshed manually.

To manage skill sources, go to **Obot Agent Management > Skills** and select the **Sources** tab.


### Adding a Source {#skills-adding-a-source}

1. Click **Add Source URL**
2. Enter a **Name** for the source
3. Provide the **Repository URL** (HTTPS)
4. Optionally specify a **Ref** (branch, tag, or commit hash) — if omitted, Obot uses the branch in the URL or `main`
5. Save the source

GitHub, GitLab, and Bitbucket Cloud URLs work with or without a `.git` suffix. For example, `https://bitbucket.org/workspace/repo` is a valid source. Self-hosted git repositories require a `.git` suffix. See [supported Git URL formats and private repository authentication](../configuration/mcp-server-gitops.md#supported-url-formats) for details.

After saving, Obot fetches the repository and discovers skills. The sync status appears next to the source entry, showing whether the sync is in progress, how many skills were found, or any errors that occurred.

> Try out `https://github.com/obot-platform/skills` to access some examples.


### Refreshing a Source {#skills-refreshing-a-source}

Sources sync automatically every hour, but you can trigger an immediate sync by selecting a source and clicking the **Sync** button. This is useful after pushing changes to a skill repository.


### Removing a Source {#skills-removing-a-source}

Deleting a source also removes all skills that were discovered from it. Users who previously installed those skills keep their local copies, but the skills will no longer appear in search results.


## Browsing Skills {#skills-browsing-skills}

To view all discovered skills, go to **Obot Agent Management > Skills** and select the **Skills** tab.

The skills list shows every valid skill found across all configured sources. Each entry displays the skill's name, description, creation date, and which source it came from. You can:

- **Search** skills by name or description
- **Filter** by source repository
- **Click a skill** to view its full metadata, including repository URL, commit reference, license, and compatibility

Skills in this view are read-only. Their content is managed in the source git repository—to update a skill, push changes to the repository and sync the source.

:::note
Skills that fail validation (for example, due to a malformed `SKILL.md`) still appear in the list but are marked with a warning icon and a description of the validation error.
:::


## How Agents Use Skills {#skills-how-agents-use-skills}

When agents are running in Obot, they have built-in tools for working with skills:

- **Search skills** — Agents can search the skill catalog to find skills matching a query. Only skills the current user has access to (based on [Skill Access Policies](../functionality/skill-access-policies.md)) are returned.
- **Install a skill** — Agents can download and install a skill from the catalog. If a skill with the same name is already installed, the agent asks for confirmation before overwriting it.
- **List installed skills** — Agents can see all skills currently available to them, including both built-in skills and user-installed ones.
- **Use a skill** — Once installed, an agent can read and follow the skill's instructions during a conversation.

Agents also come with a small set of built-in skills (such as workflow management and Python scripting) that are always available without installation.


### Example Interaction {#skills-example-interaction}

A typical skill workflow in chat looks like:

1. A user asks the agent to find and install a code review skill
2. The agent searches for a relevant skill (e.g., a "code-review" skill)
3. The agent installs the skill
4. The user asks the agent to review some code
5. The agent loads the skill and follows its instructions during the code review

Once installed, a skill remains available for future conversations without needing to be installed again.
Skills are installed at the agent level, so they are available in all conversation threads.


## Access Control {#skills-access-control}

By default, skills are not visible to regular users. Administrators must create [Skill Access Policies](../functionality/skill-access-policies.md) to grant users and groups access to specific skills or entire skill sources.

Administrators always have full access to all skills regardless of policies.

## Overview {#skill-access-policies-overview}

Skill Access Policies control which users and groups can discover and install which skills. Administrators create policies to grant skill access based on organizational needs—whether that means giving everyone access to all skills, restricting certain skill sources to specific teams, or granting access to individual skills.

Without a policy granting access, regular users cannot see or install any skills. Administrators always have full access regardless of policies.


## How Policies Work {#skill-access-policies-how-policies-work}

Each policy defines two things:

- **Who** can access the skills (users and groups)
- **Which** skills they can access

When an agent searches for skills on behalf of a user, only skills granted through one or more policies are returned. If no policy grants a user access to any skills, the agent's skill search returns no results.


### Users and Groups {#skill-access-policies-users-and-groups}

A policy can grant access to:

- **Individual users** — Select specific people by name
- **Groups** — Select authentication provider groups (such as "engineering" or "data-science")
- **Everyone** — Use the "All Users" option to grant access to all authenticated users

Using "All Users" is convenient for making a baseline set of skills universally available, while separate policies can grant additional skills to specific teams.


### Skills {#skill-access-policies-skills}

When adding skills to a policy, you can select:

- **Individual skills** — Specific skills by name
- **Entire skill sources** — All skills from a particular source repository, including any skills added to that repository in the future
- **All skills** — Grants access to every skill across all sources, including skills added in the future

Skills are displayed grouped by their source repository, making it easy to find and select related skills.


## Managing Policies {#skill-access-policies-managing-policies}

To manage policies, go to **Obot Agent Management > Skill Access Policies**.


### Creating a Policy {#skill-access-policies-creating-a-policy}

1. Click **Create Policy**
2. Enter a descriptive **Name** for the policy
3. Add **Users & Groups** — search for and select the users or groups who should have access
4. Add **Skills** — search for and select individual skills, entire skill sources, or all skills
5. Click **Create**


### Editing a Policy {#skill-access-policies-editing-a-policy}

Click any policy in the list to view and modify its name, users and groups, or skills. Changes take effect immediately.


### Deleting a Policy {#skill-access-policies-deleting-a-policy}

Deleting a policy removes skill access for the affected users. If a user loses access to all skills as a result, agents acting on their behalf will no longer be able to search for or install skills until another policy grants them access.

Skills that were already installed before losing access remain available in the agent session.


## Example: Data Science Team {#skill-access-policies-example-data-science-team}

To give your data science team access to data-related skills:

1. Create a policy named "Data Science Skills"
2. Add the "data-science" group as a subject
3. Add the skill source repository that contains your data analysis and visualization skills
4. Save the policy

Members of the data science group can now search for and install any skill from that repository. As new skills are added to the repository, they automatically become available to the team.


## Multiple Policies {#skill-access-policies-multiple-policies}

Access is additive across policies. If a user matches multiple policies, they get access to the combined set of skills from all matching policies. There is no way to deny access through a policy—policies only grant access.


## Related Topics {#skill-access-policies-related-topics}

- [Skills](../functionality/skills.md) — Manage skill sources and browse available skills
