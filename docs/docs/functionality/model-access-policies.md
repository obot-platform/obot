---
title: "Control model access"
---

## Overview

Model Access Policies control which users and groups can use which language models in Obot Agent and through the LLM Gateway. Administrators create policies to grant model access based on organizational needs—whether that means giving everyone access to standard models, restricting powerful models to specific teams, or anything in between.

For external clients, the gateway's model list and requests are limited by these grants. A provider being configured does not make its models available to every user. See [LLM Gateway](../llm-gateway/how-it-works.md) for provider routes and model discovery.

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
- **Default model aliases** — References to whichever model is currently set as the default for a given purpose (see [Default Model Aliases](./model-access-policies.md#default-model-aliases))
- **Wildcard suffix patterns** — Grants access to every model whose ID starts with a given prefix (see [Wildcard Suffix Patterns](./model-access-policies.md#wildcard-suffix-patterns))
- **All models** — Grants access to every available model

:::info Administrators Must Follow Policies
Administrators do not have automatic access to all models. Like any other user, an administrator must be included in a policy to use a model in Obot Agent.
:::

#### Wildcard Suffix Patterns

To grant access to a family of models—including versions that don't exist yet—end your entry with a single `*`. For example, `claude-haiku-4-5*` grants access to every model whose provider-native ID starts with `claude-haiku-4-5`, such as date-suffixed releases like `claude-haiku-4-5-20251001`.

To add a pattern, start typing a prefix into the search box of the **Add Models** dialog. A pattern entry (your prefix followed by `*`) is offered at the top of the results, along with a live count of the models it currently matches. You can also type the pattern out explicitly, ending with `*`.

Patterns follow these rules:

- The `*` is only allowed at the end of the entry, and the prefix cannot be empty or begin/end with whitespace (a bare `*` is the existing "All models" option)
- Matching is case-sensitive and applies to the provider-native model ID
- Patterns match models from **all** providers
- Patterns automatically cover future models: when a provider adds a new model whose ID starts with the prefix, users gain access without any policy changes
- Patterns can be combined with specific models and default model aliases in the same policy

## Model Availability

Only models configured with the **Language Model (Chat)** usage type appear when creating policies. Models configured for other purposes—such as text embedding, image generation, or vision—do not appear as options.

To change which models are available for chat or to configure new model providers, see [Model Providers](../configuration/model-providers.md).

## Default Model Aliases

Default aliases are **global to the Obot installation**. A policy can grant access through an alias, but cannot choose or override the model that alias points to.

| Policy selection | Alias | Purpose |
|---|---|---|
| Language Model (Chat) | `llm` | Primary default language model |
| Language Model (Chat - Fast) | `llm-mini` | Default for faster language-model tasks |

Administrators change the targets under **Models > Model Providers > Set Default Models**. Selecting an alias in a policy grants its subjects access to whichever model that global alias currently resolves to.

For example, if a policy grants `llm` and an administrator changes `llm` from model A to model B, the policy's users gain access to B. That alias no longer grants A; users retain access to A only if another matching grant allows it. To keep a team's access tied to a particular model, select that specific model in the policy.

Clients may also use `llm` and `llm-mini` in LLM Gateway requests. These request aliases use the same global targets and still require access to the resolved model and its matching provider route. See [Using model aliases](../llm-gateway/how-it-works.md#using-model-aliases).

## Fresh Installations

When Obot starts for the first time, a **Default Policy** is automatically created. This policy:

- Grants access to **All Obot Users**
- Includes all default model aliases

This ensures that once a provider and its default model aliases are configured, users covered by the policy can access those defaults. You can modify or delete this policy to restrict access as needed.

## Upgrades and Migration

For existing installations that previously used **Allowed Models** and **Default Model** in Chat Configuration:

- A **Migrated Policy** is automatically created
- Your previous allowed models are preserved in this policy
- Your previous default model setting is preserved as the default model alias
- No action is required

You can find and modify this migrated policy on the Model Access Policies page. The previous settings in Chat Configuration no longer control model access.

## Managing Policies

To manage policies, go to **Models > Access Policies**.

### Creating a Policy

1. Click **Add Access Policy**
2. Enter a descriptive name
3. Add subjects (users, groups, or All Obot Users)
4. Select which models to include
5. Save the policy

### Editing a Policy

Click any policy in the list to modify its name, subjects, or models. Changes take effect immediately.

### Deleting a Policy

Deleting a policy removes model access for the affected subjects. If a user loses access to all models as a result, they will no longer be able to use chat until another policy grants them access.

## Related Topics

- [Model Providers](../configuration/model-providers.md) — Configure language models and set defaults
- [MCP Access Policies](../mcp-gateway/access.md) — Similar access control for MCP servers
- [User Roles](../security/policy-coverage.md) — Understanding administrator and user permissions
