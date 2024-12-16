---
title: Overview
slug: /
---
import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

Acorn is an open source enterprise agent platform. Key features include:
- Ability to build agents to support a variety of usecases including copilots, assistants, and autonomous agentic workflows.
- Integration with leading LLM providers
- Built-in RAG for your data
- Easy integration with custom or private web services and APIs
- OAuth 2.0 authentication

### Getting Started
Launch Acorn via docker:
```bash
docker run -d -p 8080:8080 -e "OPENAI_API_KEY=<OPEN AI KEY>" ghcr.io/acorn-io/acorn:latest
```
Then visit http://localhost:8080.

The `acorn` CLI can be installed via brew on MacOS or Linux:
```bash
brew tap acorn-io/tap
brew install acorn
```
or by downloading the binary for your platform from our [latest release](https://github.com/acorn-io/acorn/releases/latest).
