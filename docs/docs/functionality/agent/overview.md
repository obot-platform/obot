# Obot Agent

Obot Agent is a web-based interface for interacting with AI. Users customize their agent then chat with their agent through conversations.

## Conversations

Conversations are isolated message sessions with your agent. Each conversation has:

- **Isolated conversation history**: Messages don't appear in other conversations
- **Independent credentials**: Tool authentication is conversation-specific
- **Access to shared resources**: Knowledge, memory, and project files

Create a new conversation to start fresh while maintaining the same agent configuration.

## Workflows

Workflows automate interactions through scheduled or on-demand execution.

- **Scheduled**: Run on recurring schedules (hourly, daily, weekly)
- **On-demand**: Trigger manually or via API
- **Parameterized**: Accept inputs to customize behavior

Workflows use the same configuration and credentials as the parent agent.

### Creating a Workflow

1. Open your agent
2. Start a new Conversation
3. Select 'Create a workflow' from the options
4. Chat with your agent to define the workflow

### Sharing Workflows

Workflows can also be published as reusable shared packages so other users can discover and install them. Shared workflows preserve the workflow's `SKILL.md` metadata and supporting files, and they use versioning so you can republish improvements over time.

See [Workflow Sharing](../workflow-sharing.md) for the publishing, search, installation, storage, and access-control details.

## MCP Server Connections

Connect MCP servers to your agent to enable powerful tools and capabilities. Simply chat with your agent to configure server connections.

- All MCP traffic flows through the gateway for access control and logging


:::note
To re-enable the legacy Obot Chat interface, you can set the `OBOT_SERVER_DISABLE_LEGACY_CHAT` environment variable to `false`. This will allow you to temporarily access the old chat interface, but that functionality will be removed in a future release.
