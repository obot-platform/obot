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

:::note Model Access
Model availability for chat is now controlled through [Model Access Policies](../model-access-policies/). The previous "Allowed Models" setting has been replaced by policies that let you control which users and groups can access which models.
:::

## Model Providers

Configure LLM providers and their available models. See [Model Providers](/configuration/model-providers/) for setup details.

## Model Access Policies

Control which users and groups can access which models for chat. See [Model Access Policies](../model-access-policies/) for details.

## Chat Threads, Tasks, and Task Runs

Administrators can list and view chat threads, tasks, and task runs for all users across the platform.

Only users with the Auditor role can view the full details of chat threads and task runs. Other administrators see metadata only.
