---
title: Model Access Policies
---

## Overview

Model Access Policies control which users and groups can use which language models in chat. Administrators create policies to grant model access based on organizational needs—whether that means giving everyone access to standard models, restricting powerful models to specific teams, or anything in between.

This feature replaces the previous **Allowed Models** and **Default Model** settings that were part of Chat Configuration. If you previously used those settings, see [Upgrades and Migration](#upgrades-and-migration) for how your configuration was preserved.


## How Policies Work

Each policy defines two things:

- **Who** can use the models (subjects)
- **Which** models they can use

When a user opens chat, they see only the models granted to them through one or more policies. If no policy grants a user access to any models, they cannot use chat.

### Subjects

A policy can grant access to:

- **Individual users** — Select specific people by name
- **Groups** — Select authentication provider groups (such as "engineering" or "marketing")
- **Everyone** — Use the "All Obot Users" option to grant access to all authenticated users

Using "All Obot Users" is convenient for making certain models universally available while using separate policies to grant additional models to specific teams.

### Models

When adding models to a policy, you can select:

- **Specific models** — Individual models from your configured providers
- **Default model aliases** — References to whichever model is currently set as the default for a given purpose (see [Default Model Aliases](#default-model-aliases))
- **All models** — Grants access to every available model

:::info Administrators Must Follow Policies
Administrators do not have automatic access to all models. Like any other user, an administrator must be included in a policy to use a model in chat.
:::

## Model Availability

Only models configured with the **Language Model (Chat)** usage type appear when creating policies. Models configured for other purposes—such as text embedding, image generation, or vision—do not appear as options.

To change which models are available for chat or to configure new model providers, see [Model Providers](/configuration/model-providers).

## Default Model Aliases

When selecting models for a policy, you'll see options like:

- **Language Model (Chat)** — The primary default model
- **Language Model (Chat - Fast)** — A faster, typically smaller model

These are aliases that automatically resolve to whichever model is currently configured as the default in [Model Providers](/configuration/model-providers).

Using aliases provides flexibility: if you later change which model serves as the default, users with access to the alias automatically gain access to the new default without needing to update any policies.

## Fresh Installations

When Obot starts for the first time, a **Default Policy** is automatically created. This policy:

- Grants access to **All Obot Users**
- Includes all default model aliases

This ensures that once you configure a model provider, all users can immediately start chatting. You can modify or delete this policy to restrict access as needed.

## Upgrades and Migration

For existing installations that previously used **Allowed Models** and **Default Model** in Chat Configuration:

- A **Migrated Policy** is automatically created
- Your previous allowed models are preserved in this policy
- Your previous default model setting is preserved as the default model alias
- No action is required

You can find and modify this migrated policy on the Model Access Policies page. The previous settings in Chat Configuration no longer control model access.

## Managing Policies

To manage policies, go to **Chat Management > Model Access Policies**.

### Creating a Policy

1. Click **Create Policy**
2. Enter a descriptive name
3. Add subjects (users, groups, or All Obot Users)
4. Select which models to include
5. Save the policy

### Editing a Policy

Click any policy in the list to modify its name, subjects, or models. Changes take effect immediately.

### Deleting a Policy

Deleting a policy removes model access for the affected subjects. If a user loses access to all models as a result, they will no longer be able to use chat until another policy grants them access.

## Related Topics

- [Model Providers](/configuration/model-providers) — Configure language models and set defaults
- [MCP Registries](/functionality/mcp-registries/) — Similar access control for MCP servers
- [User Roles](/configuration/user-roles) — Understanding administrator and user permissions
