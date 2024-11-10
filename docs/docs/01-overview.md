---
title: Overview
slug: /
---
import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

Otto8 is an open source enterprise agent platform. Key features include:
- Ability to build agents to support a variety of usecases including copilots, assistants, and autonomous agentic workflows.
- Integration with leading LLM providers
- Built-in RAG for your data
- Easy integration with custom or private web services and APIs
- OAuth 2.0 authentication

### Getting Started
Launch Otto8 via docker:
```bash
docker run -d -p 8080:8080 -e "OPENAI_API_KEY=<OPEN AI KEY>" ghcr.io/otto8-ai/otto8:latest
```
Then visit http://localhost:8080.

The `otto8` CLI can be installed via brew on MacOS or Linux:
```bash
brew tap otto8-ai/tap
brew install otto8
```
or by downloading the binary for your platform from our [latest release](https://github.com/otto8-ai/otto8/releases/latest).
