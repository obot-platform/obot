# Obot Chat

Obot Chat is a web-based interface for interacting with AI through customizable projects. Users create projects configured with instructions, knowledge, and MCP server connections, then chat with them through threads.

## Projects

A project combines an LLM, instructions, and connectors to create a specialized assistant. Each project has:

- **Instructions**: Define the assistant's behavior and personality
- **Knowledge**: Documents uploaded for RAG queries
- **Connectors**: MCP servers for external integrations
- **Memory**: Important information persisted across all threads
- **Model**: The LLM provider and model to use

### Creating a Project

1. Click **New Project** in the sidebar
2. Give your project a name
3. Add instructions describing what the assistant should do
4. Configure connectors by enabling MCP servers from the registry
5. Optionally upload knowledge files

### Sharing Projects

Publish a project as a template for other users to create their own independent copies with their own threads, memory, and MCP server configuration.

## Threads

Threads are individual conversations within a project. Each thread has:

- **Isolated conversation history**: Messages don't appear in other threads
- **Independent credentials**: Tool authentication is thread-specific
- **Access to shared resources**: Knowledge, memory, and project files

Create new threads to start fresh conversations while maintaining the same project configuration.

## Tasks

Tasks automate project interactions through scheduled or on-demand execution.

- **Scheduled**: Run on recurring schedules (hourly, daily, weekly)
- **On-demand**: Trigger manually or via API
- **Parameterized**: Accept inputs to customize behavior

Tasks use the same connectors and knowledge as the parent project.

### Creating a Task

1. Open a project
2. Navigate to the Tasks tab
3. Click **New Task**
4. Define the task prompt and any input parameters
5. Optionally configure a schedule

## MCP Server Connections

Connect to MCP servers through your projects:

- Browse available servers from the registry
- Configure credentials per server
- Enable or disable servers per project
- All MCP traffic flows through the gateway for access control and logging

## Model Providers

Admins and Owners configure which LLM providers and models are available to users. Users select from these configured models while chatting.

See [Model Providers](/configuration/model-providers/) for configuration details.

## Knowledge (RAG)

Add domain-specific information to your conversations:

- **File Upload**: Upload documents, PDFs, and text files
- **Smart Retrieval**: Automatically finds relevant information for queries
- **Project-Scoped**: Knowledge is shared across all threads in a project

## Memory

Maintain important context across conversations:

- **Project Memory**: Key information remembered across all threads
- **Automatic Extraction**: Important details captured from conversations
- **User Control**: Review and manage stored memories

## Getting Started

1. **Create a Project**: Define what the assistant should do
2. **Add Connectors**: Enable MCP servers for the project
3. **Upload Knowledge** (optional): Add documents for context
4. **Start Chatting**: Open a thread and interact with your assistant
