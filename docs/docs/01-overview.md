---
title: Overview
slug: /
---
import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

Obot is an open source enterprise agent platform. Key features include:

- Ability to build agents to support a variety of usecases including copilots, assistants, and autonomous agentic workflows.
- Integration with leading LLM providers
- Built-in RAG for your data
- Easy integration with custom or private web services and APIs
- OAuth 2.0 authentication

### Quick Start

Launch Obot via Docker:

```bash
docker run -d -p 8080:8080 -e "OPENAI_API_KEY=<OPENAI KEY>" ghcr.io/obot-platform/obot:main
```

Then visit [http://localhost:8080](http://localhost:8080).

The `Obot` CLI can be installed via brew on macOS or Linux:

```bash
brew tap obot-platform/tap
brew install obot
```

or by downloading the binary for your platform from our [latest release](https://github.com/obot-platform/obot/releases/latest).

### Installation

See [Installation](/installation/installation) for more information on how to install Obot.
