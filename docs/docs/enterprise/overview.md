---
title: "Editions and feature availability"
---

# Editions and feature availability {#obot-editions}

Obot is available in three editions. They are all delivered in the same container image, and you can
upgrade from one to the next in the app. The editions differ in which capabilities are enabled and
what usage limits apply.

## Obot

The default edition supports up to 100 users and 100 devices. It includes Local, GitHub, and Google authentication and all [model providers](../configuration/model-providers.md), including Azure OpenAI, Amazon Bedrock, and Google Vertex, without registration. Entra, Okta, JumpCloud, and Auth0 authentication require Community registration or an Enterprise license.

## Obot Community

Adds the enterprise-grade [auth providers](../configuration/auth-providers.md), Entra, Okta, JumpCloud, and
Auth0, so your users can log in through your existing identity provider. The 100 user and device limits still apply.

Obot Community is free and just requires a one-time registration. To enable it, go to the **License** page in the admin UI.

## Obot Enterprise

Everything in Obot Community, with unlimited users and devices and enterprise support. To get an Enterprise license,
[contact us](https://obot.ai/contact-us/).

## Feature availability

Edition limits and runtime availability are separate. A license does not enable a disabled feature or supply its infrastructure.

| Capability | Additional requirement or status |
|---|---|
| MCP and LLM gateways, registries, and skills | Configure resources, credentials, and access policies for the intended users |
| Obot Agent and workflows | Disabled on new installations unless `OBOT_ENABLE_AGENTS=true`; existing agent installations stay enabled when unset |
| Hosted Agents | Requires `OBOT_SERVER_ENABLE_HOSTED_AGENTS=true` and a configured Kubernetes sandbox backend; no Docker backend exists |
| Device Management | Beta; install and enroll Obot Sentry on supported devices |
| Local tool-call enforcement | Experimental; only supported clients and identified tool calls are covered |
| AI Judge Policies | Experimental; requires `OBOT_SERVER_ENABLE_MESSAGE_POLICIES=true` and configured review models |
| Managed image pull secrets and Kubernetes secret bindings | Kubernetes MCP runtime required |
| Domain-based MCP egress control | Kubernetes and a configured supported policy provider required |

See [Agent availability](../agents/availability.md), [Device Management](../device-management/how-sentry-works.md), [AI Judge Policies](../functionality/ai-judge-policies.md), and [Server Configuration](../configuration/server-configuration.md) for the relevant constraints.
