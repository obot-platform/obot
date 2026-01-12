---
title: Obot Chat
---

# Obot Chat

Obot Chat is a chat interface built to work directly with MCP. It provides a conversational way for users to interact with MCP servers and accomplish tasks using AI.

## Key Concepts

### Projects

Projects are the primary organizational unit. Each project combines an LLM with instructions, knowledge, and MCP server connections to create a specialized assistant.

### Threads

Threads are individual conversations within a project. Each thread has isolated conversation history while sharing the project's configuration and resources.

### Tasks

Tasks automate project interactions through scheduled or on-demand execution. They can run on recurring schedules or be triggered manually.

### Model Providers

Obot Chat supports multiple LLM providers including OpenAI, Anthropic, Azure OpenAI, and Amazon Bedrock. Model providers are configured at the platform level and made available to users.

### Knowledge

Projects can include uploaded documents for RAG queries. The system automatically retrieves relevant information during conversations.

### Memory

Projects can persist important information across all threads, capturing key details from conversations for future reference.

## Learn More

- [Obot Chat](/functionality/chat/overview) - Detailed configuration and usage
