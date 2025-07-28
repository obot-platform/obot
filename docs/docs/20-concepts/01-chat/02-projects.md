# Projects

A project is an AI agent that combines an LLM, a set of instructions, and MCP Servers to perform tasks, answer questions, and interact with its environment. Projects are the core building blocks of the obot chat interface.

## Key Concepts

### What is a Project?

Think of a project as a specialized AI assistant designed for specific tasks or domains. Each project can be customized with:

- **Instructions (System Prompt)**: Define the agent's personality, goals, and behavior
- **Knowledge**: Upload documents and data for the agent to reference
- **Tools**: Enable specific capabilities through MCP server integrations
- **Model Configuration**: Choose which LLM providers and models to use
- **Access Control**: Determine who can use and modify the project

### Memories

As you chat with your project, you can ask it to remember important information. These memories are:
- **Persistent**: Stored across all threads in the project
- **Contextual**: Added to the system prompt for all conversations
- **Manageable**: View, edit, and remove memories through the UI
- **Shareable**: Available to all users who have access to the project

## Configuration

### Name and Description

These fields help users identify and understand the project's purpose. Make them clear and descriptive.

### Instructions

The Instructions guide your project's behavior. Use this to specify:
- **Objectives**: What the project is meant to accomplish
- **Personality**: How the agent should communicate
- **Domain Knowledge**: Special context about users or use cases
- **Constraints**: Any limitations or guidelines to follow

Example for an HR Assistant:
> You are an HR assistant for Acme Corporation. Employees will chat with you to get answers to HR-related questions. If an employee seems unsatisfied with your answers, direct them to email `hr@acmecorp.com`.

### Built-In Capabilities

Control which core capabilities are enabled:

| Capability | Description | Default |
|------------|-------------|---------|
| Memory | Allow the LLM to remember important information across threads | Enabled |
| Knowledge | Use provided files to perform RAG queries | Enabled |
| Time | Provide current date, time, and timezone information | Enabled |

### Knowledge Configuration

The knowledge capability lets you provide project-specific information through RAG:

- **File Upload**: Upload documents, PDFs, spreadsheets directly

### Project Files

Project files are shared across all threads in a project. They're useful for:
- **Templates**: Standard documents the agent can reference or modify
- **State Management**: Persistent data that survives across conversations
- **Collaboration**: Files that multiple threads can read and update

### Model Providers

Configure which LLM providers and models are available:
- **Default Model**: The primary model for conversations
- **Model Options**: Allow users to choose from multiple models
- **Provider Settings**: API keys, endpoints, and configuration

### Members and Access Control

Manage who can access your project:
- **Invitations**: Send invites to specific users
- **Thread Privacy**: Users can access all threads but cannot modify project configuration

:::warning Security Note
All users with project access can access thread history. Be careful not to share sensitive information.
:::

## Best Practices

### Effective System Prompts
1. **Be Specific**: Clearly define the agent's role and responsibilities
2. **Provide Context**: Include relevant background information
3. **Set Boundaries**: Specify what the agent should and shouldn't do
4. **Include Examples**: Show the desired communication style

### Knowledge Management
1. **Organize Files**: Use clear, descriptive filenames
2. **Update Regularly**: Keep knowledge current and relevant
3. **Describe Purpose**: Write helpful knowledge descriptions
4. **Test Retrieval**: Verify the agent can find relevant information

### Tool Selection
1. **Start Simple**: Begin with essential tools only
2. **Add Gradually**: Introduce new tools as needs become clear
3. **Test Integration**: Verify tools work correctly with your use case
4. **Document Usage**: Help users understand available capabilities
