# Chat Overview

The Chat Interface provides a web-based conversational interface where users can chat with AI agents (called Projects) to accomplish tasks, get answers, and collaborate on work. The Chat Interface makes AI agents accessible and powerful while maintaining the familiar experience of conversational interaction.

## Core Components

### Projects (AI Agents)
Projects are individual AI assistants that can be customized for specific tasks or domains. Each project has:
- **System Prompt**: Instructions that define the agent's behavior and personality
- **Knowledge Base**: Access to uploaded documents and data through RAG
- **Tool Access**: Ability to use external tools and services through MCP
- **Model Configuration**: Choice of LLM providers and models
- **Sharing Settings**: Control who can access and use the project

### Threads
Threads represent individual conversations within a project. Key characteristics:
- **Isolated Context**: Each thread maintains its own conversation history
- **Shared Memory**: Important information can be remembered across threads
- **Credentials**: Each thread has its own authentication context for tools
- **Workspace**: Threads can share files within the project workspace

### Knowledge Integration
Built-in Retrieval Augmented Generation (RAG) allows projects to work with your data:
- **File Upload**: Upload documents, PDFs, text files directly
- **External Sources**: Connect to Notion, OneDrive, websites
- **Smart Retrieval**: Automatically finds relevant information for queries
- **Knowledge Description**: Help the agent understand when to use knowledge

### Tools and MCP Integration
Projects can interact with external systems through the Model Context Protocol:
- **Browse the Web**: Access and analyze web content
- **Send Emails**: Draft and send email messages  
- **API Calls**: Interact with REST APIs and web services
- **File Operations**: Create, edit, and manage files in workspaces
- **Custom Tools**: Connect any MCP-compatible tool or service

## User Workflows

### Basic Chat
1. Select or create a project
2. Start a new thread or continue an existing conversation
3. Chat naturally with the AI agent
4. The agent uses its knowledge and tools to help accomplish tasks

### Project Creation
1. Define the project's purpose and instructions
2. Upload relevant knowledge/documents
3. Configure available tools and integrations
4. Set sharing permissions
5. Start chatting with your customized agent

### Collaboration
1. Share projects with team members
2. Multiple users can create their own threads
3. Shared knowledge and memories benefit all users
4. Project owners can modify configuration

## Configuration

For more information on configuring chat, visit the [chat configuration](/configuration/chat-configuration) documentation.