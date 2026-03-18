---
title: Skills
---

## Overview

Skills are reusable, structured instructions that agents can discover and install to expand their capabilities. Each skill is a self-contained package stored in a GitHub repository, containing a `SKILL.md` file with a description, metadata, and the instructions themselves. Obot indexes skills from configured sources and makes them available to agents based on access policies.

Administrators manage skill sources and control who can access which skills. Agents search, install, and use skills during conversations.

## What Is a Skill?

A skill is a directory in a GitHub repository that contains a `SKILL.md` file. This file uses YAML frontmatter to define the skill's identity—its name, description, license, and compatibility requirements—followed by markdown content with the actual instructions.

Skills can also include supporting files alongside the `SKILL.md`, such as helper scripts or reference data. When an agent installs a skill, the entire directory is downloaded.

Each skill has:

- **Name** — A unique identifier within the repository (e.g., `code-review`)
- **Display Name** — A human-readable name generated from the skill name (e.g., "Code Review")
- **Description** — A summary of what the skill does
- **License** — The skill's license, if specified
- **Compatibility** — Any requirements or constraints (e.g., "Python 3.8+")

Obot follows the Agent Skills standard. See [agentskills.io](https://agentskills.io) for more information about skills.

## Skill Sources

Skill sources are GitHub repositories that contain one or more skills. When you add a source, Obot scans the repository for directories containing `SKILL.md` files and indexes each valid skill it finds. Sources are synced automatically every hour and can also be refreshed manually.

To manage skill sources, go to **Obot Agent Management > Skills** and select the **Sources** tab.

### Adding a Source

1. Click **Add Source URL**
2. Enter a **Name** for the source
3. Provide the **GitHub URL** of the repository (must be an HTTPS GitHub URL)
4. Optionally specify a **Ref** (branch, tag, or commit hash) — if omitted, the default branch is used
5. Save the source

After saving, Obot fetches the repository and discovers skills. The sync status appears next to the source entry, showing whether the sync is in progress, how many skills were found, or any errors that occurred.

> Try out `https://github.com/obot-platform/skills` to access some examples.

### Refreshing a Source

Sources sync automatically every hour, but you can trigger an immediate sync by selecting a source and clicking the **Sync** button. This is useful after pushing changes to a skill repository.

### Removing a Source

Deleting a source also removes all skills that were discovered from it. Users who previously installed those skills keep their local copies, but the skills will no longer appear in search results.

## Browsing Skills

To view all discovered skills, go to **Obot Agent Management > Skills** and select the **Skills** tab.

The skills list shows every valid skill found across all configured sources. Each entry displays the skill's name, description, creation date, and which source it came from. You can:

- **Search** skills by name or description
- **Filter** by source repository
- **Click a skill** to view its full metadata, including repository URL, commit reference, license, and compatibility

Skills in this view are read-only. Their content is managed in the source GitHub repository—to update a skill, push changes to the repository and sync the source.

:::note
Skills that fail validation (for example, due to a malformed `SKILL.md`) still appear in the list but are marked with a warning icon and a description of the validation error.
:::

## How Agents Use Skills

When agents are running in Obot, they have built-in tools for working with skills:

- **Search skills** — Agents can search the skill catalog to find skills matching a query. Only skills the current user has access to (based on [Skill Access Policies](../skill-access-policies/)) are returned.
- **Install a skill** — Agents can download and install a skill from the catalog. If a skill with the same name is already installed, the agent asks for confirmation before overwriting it.
- **List installed skills** — Agents can see all skills currently available to them, including both built-in skills and user-installed ones.
- **Use a skill** — Once installed, an agent can read and follow the skill's instructions during a conversation.

Agents also come with a small set of built-in skills (such as workflow management and Python scripting) that are always available without installation.

### Example Interaction

A typical skill workflow in chat looks like:

1. A user asks the agent to find and install a code review skill
2. The agent searches for a relevant skill (e.g., a "code-review" skill)
3. The agent installs the skill
4. The user asks the agent to review some code
5. The agent loads the skill and follows its instructions during the code review

Once installed, a skill remains available for future conversations without needing to be installed again.
Skills are installed at the agent level, so they are available in all conversation threads.

## Access Control

By default, skills are not visible to regular users. Administrators must create [Skill Access Policies](../skill-access-policies/) to grant users and groups access to specific skills or entire skill sources.

Administrators always have full access to all skills regardless of policies.
