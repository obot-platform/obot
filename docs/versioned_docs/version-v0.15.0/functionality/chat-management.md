---
title: Chat Management
---

# Chat Management

Chat Management provides administrators with tools to configure default chat settings and monitor chat activity. Access these features from **Chat Management** in the sidebar.

## Chat Configuration

Configure default settings that apply to all new projects. Changes here affect the starting point for user-created projects.

- **Name**: The default assistant name shown in the chat interface
- **Description**: A brief description of the default assistant
- **Introductions**: HTML content displayed when users start a new thread
- **Instructions**: Default system prompt defining assistant behavior
- **Allowed Models**: Models available to users, with a designated default

### Allowed Models

Control which models users can select while chatting:

1. Click **+ Add Model** to add models from configured providers
2. Set a default model by clicking the three-dot menu and selecting **Set as Default**
3. Remove models by clicking the three-dot menu and selecting **Remove**

## Model Providers

Configure LLM providers and their available models. See [Model Providers](/configuration/model-providers) for setup details.

## Chat Threads, Tasks, and Task Runs

Administrators can list and view chat threads, tasks, and task runs for all users across the platform.

Only users with the Auditor role can view the full details of chat threads and task runs. Other administrators see metadata only.
