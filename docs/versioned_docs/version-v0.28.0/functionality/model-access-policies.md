---
title: "Profiles: model access, budgets and guardrails"
---

## Overview

Profiles decide who gets what from the LLM Gateway. A profile names users and groups, and gives them:

- **Models** they can use through the LLM Gateway.
- **Budgets** that cap their spend, tokens or requests.
- **Applied guardrails** that check their model requests and responses. See [Guardrails](./guardrails.md).

Administrators create profiles for organizational needs—whether that means giving everyone standard models and a monthly budget, restricting powerful models to specific teams, or checking one team's traffic for regulated data. Profiles were called model access policies, and the API still uses that name (`/api/model-access-policies`).

For external clients, the gateway's model list and requests are limited by these grants. A provider being configured does not make its models available to every user. See [Connect clients and applications](../llm-gateway/connect-clients.md) for how clients connect and list their models.

## How Profiles Work

Each profile defines:

- **Who** it applies to (subjects)
- **Which** models they can use. A profile can grant no models, to only apply guardrails. Its budgets then never apply, since a budget covers only its own profile's models.
- Its **budgets** and **applied guardrails**

Profiles add up. A user in several profiles can use the models of each, and the applied guardrails of each check their traffic. A budget covers only its own profile's models: a request is limited by the budgets of the user's profiles that grant the model that serves it (all of a profile's models, or the specific models a budget names). Every one of those budgets must have room, so when several profiles' budgets cover the same model, the tightest one limits it. One profile can't lift another's limits. Routing rules and fallbacks can serve a request with a model other than the one requested, so budgets are checked for each model the gateway tries, and the request is charged to the model that serves it. Requests paid for by the user's own Claude subscription don't count against budgets.

For example, if *Everyone* grants model A with a $10 monthly budget and *Power users* grants models A and B with a $100 monthly budget, a power user's requests for A must fit both budgets, and their requests for B only the *Power users* budget.

The LLM Gateway page and gateway model discovery show only models granted to the user through one or more profiles. If no profile grants access to a model, the user cannot call it through the LLM Gateway.

### Subjects

A profile can grant access to:

- **Individual users** — Select specific people by name
- **Groups** — Select authentication provider groups (such as "engineering" or "marketing")
- **Everyone** — Use the "All Obot Users" option to grant access to all authenticated users

Using "All Obot Users" is convenient for making certain models universally available while using separate profiles to grant additional models to specific teams.

### Models

When adding models to a profile, you can select:

- **Specific models** — Individual models from your configured providers
- **Default model aliases** — References to whichever model is currently set as the default for a given purpose (see [Default Model Aliases](./model-access-policies.md#default-model-aliases))
- **Wildcard suffix patterns** — Grants access to every model whose ID starts with a given prefix (see [Wildcard Suffix Patterns](./model-access-policies.md#wildcard-suffix-patterns))
- **All models** — Grants access to every available model

:::info Administrators Need a Profile
Administrators do not have automatic access to all models. Like any other user, an administrator must be included in a profile to use a model through the LLM Gateway.
:::

#### Wildcard Suffix Patterns

To grant access to a family of models—including versions that don't exist yet—end your entry with a single `*`. For example, `anthropic/claude-haiku-4-5*` grants access to every model whose name starts with `anthropic/claude-haiku-4-5`, such as date-suffixed releases like `anthropic/claude-haiku-4-5-20251001`.

To add a pattern, start typing a prefix into the search box of the **Add Models** dialog. A pattern entry (your prefix followed by `*`) is offered at the top of the results, along with a live count of the models it currently matches. You can also type the pattern out explicitly, ending with `*`.

Patterns follow these rules:

- The `*` is only allowed at the end of the entry, and the prefix cannot be empty or begin/end with whitespace (a bare `*` is the existing "All models" option)
- Matching is case-sensitive and applies to the model name, which starts with the provider (`provider/model`)
- To match a model family on several providers, add one pattern per provider
- Patterns automatically cover future models: when a provider adds a new model whose ID starts with the prefix, users gain access without any profile changes
- Patterns can be combined with specific models and default model aliases in the same profile

## Model Availability

Only models configured with the **Language Model (Chat)** usage type appear when creating profiles. Models configured for other purposes—such as text embedding, image generation, or vision—do not appear as options.

To change which models are available or to add providers, use **LLM Gateway > Providers**. See [Configure providers and models](../configuration/model-providers.md).

## Default Model Aliases

Default aliases are **global to the Obot installation**. A profile can grant access through an alias, but cannot choose or override the model that alias points to.

| Policy selection | Alias | Purpose |
|---|---|---|
| Language Model (Chat) | `llm` | Primary default language model |
| Language Model (Chat - Fast) | `llm-mini` | Default for faster language-model tasks |

Administrators change the targets with **Set Default Models** on the **Providers** tab of the **LLM Gateway** page; see [Default models](../llm-gateway/how-it-works.md#default-models). Selecting an alias in a profile grants its subjects access to whichever model that global alias currently resolves to.

For example, if a profile grants `llm` and an administrator changes `llm` from model A to model B, the profile's users gain access to B. That alias no longer grants A; users retain access to A only if another matching grant allows it. To keep a team's access tied to a particular model, select that specific model in the profile.

Clients may also use `llm` and `llm-mini` in LLM Gateway requests. These request aliases use the same global targets and still require access to the resolved model. See [Using model aliases](../llm-gateway/how-it-works.md#using-model-aliases).

## Fresh Installations

When Obot starts for the first time, a **Default Policy** profile is automatically created. This profile:

- Grants access to **All Obot Users**
- Includes all default model aliases
- Has a *Daily token limit* budget: per user, 10,000,000 input and 100,000 output tokens per day. Administrators are exempt, and a user's personal daily token limits replace the budget's for that user.

This ensures that once you add a provider under **LLM Gateway > Providers**, users covered by the profile can access the default models. You can modify or delete this profile to restrict access as needed.

The *Daily token limit* budget's limits come from `OBOT_SERVER_DAILY_USER_INPUT_TOKEN_LIMIT` and `OBOT_SERVER_DAILY_USER_OUTPUT_TOKEN_LIMIT`. See [Server configuration](../configuration/server-configuration.md).

## Upgrades and Migration

When upgrading from a version that used model providers, policies that named a model provider's model are re-pointed to the LLM gateway model for the same upstream model. See [Upgrading from model providers](../llm-gateway/upgrade.md). Wildcard patterns written for earlier versions matched provider-native IDs, such as `claude-haiku-4-5*`. They're replaced with the provider-qualified form, such as `anthropic/claude-haiku-4-5*`, for each provider whose models they matched. A pattern that matched no model is left as it was; add the provider to it yourself.

## Managing Profiles

To manage profiles, go to **LLM Gateway > Profiles**.

The **Profiles** tab shows each profile as a card with its users and groups and how many models, budgets and applied guardrails it has. Click a card to open the profile.

A profile is edited in one form, with a section for each part: **Name**, **Users & Groups**, **Models**, **Budgets** and **Applied Guardrails**. Budgets and applied guardrails are added and changed in a dialog. Nothing is saved until you click **Save**, which saves the profile and then each of its budgets and applied guardrails. If one of them can't be saved, the form says which and why, keeps it, and stays on the profile; fix it and save again.

### Creating a Profile

1. Click **Add Profile**
2. Enter a descriptive name
3. Add subjects (users, groups, or All Obot Users)
4. Select which models to include, if any
5. Optionally, click **Add Budget** to add budgets and **Add Guardrail** to apply guardrails
6. Click **Save**

A budget can cover all of the profile's models or only the ones you select from the models the form grants at that moment. If you later remove a model from the profile, budgets limited to it drop it; a budget left with none of its models must be edited before the profile can be saved.

### Editing a Profile

Click any profile in the list to change its name, subjects or models, and to add, change or remove its budgets and applied guardrails. Click a budget or applied guardrail to edit it, or its remove button to remove it. Changes take effect when you save.

### Deleting a Profile

Deleting a profile deletes its budgets and applied guardrails, and removes the models it grants from its users. If a user loses access to all models as a result, they will no longer be able to make model requests through the LLM Gateway until another profile grants them access.

## Related Topics

- [Configure providers and models](../configuration/model-providers.md) — Add providers and set default models
- [MCP Access Policies](../mcp-gateway/access.md) — Similar access control for MCP servers
- [User Roles](../security/policy-coverage.md) — Understanding administrator and user permissions
