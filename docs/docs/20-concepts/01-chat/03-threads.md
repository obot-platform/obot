# Threads

Threads represent individual conversations within a project. They provide context isolation while sharing project resources like knowledge, memories, and files. Understanding threads is essential for effective collaboration and context management in obot.

Threads provide the foundation for organized, collaborative AI agent interactions while maintaining clear separation between different conversations and users.

## What are Threads?

A thread is a single conversation stream between a user and an AI agent (project). Each thread maintains:

- **Conversation History**: Complete record of messages and responses
- **Context Isolation**: Separate from other threads in the same project
- **Credential Context**: Independent authentication for tools and services
- **Workspace Access**: Independent access to thread-level files

## Key Characteristics

### Context Isolation
Each thread maintains its own conversation history. This means:
- Messages from one thread don't appear in another
- Conversation context stays focused and relevant
- Multiple users can have parallel conversations
- Different topics can be explored without interference

### Shared Resources
While conversation history is isolated, threads share project-level resources:
- **Memories**: Important information remembered across all threads
- **Knowledge Base**: Access to the same uploaded documents and data
- **Project Files**: Shared workspace files accessible to all threads
- **Tool Configuration**: Same set of available MCP tools and integrations

### Independent Credentials
Each thread manages its own authentication context:
- Tool authentication is thread-specific
- Users must authenticate separately for each thread
- Prevents credential sharing between users
- Enables fine-grained access control

## Thread Lifecycle

### Creating a Thread
1. **Manual Creation**: Use the "+" button in the thread panel

### Using a Thread
1. **Start Chatting**: Begin the conversation with your first message
2. **Context Building**: Thread maintains conversation context automatically
3. **Tool Authentication**: Authenticate with tools as needed
4. **File Access**: Access and modify project or thread files within the thread

### Thread Management
- **Switch Threads**: Move between different conversations easily
- **Resume Conversations**: Return to previous threads anytime
- **Delete Threads**: Remove threads when no longer needed

## Thread vs Project Scope

Understanding what operates at the thread level vs. project level is crucial:

### Thread Level
- Conversation history and context
- Tool authentication and credentials
- User-specific settings and preferences
- Temporary files and scratch work

### Project Level
- System prompt and agent configuration
- Knowledge base and uploaded documents
- Memories and persistent information
- Project files and shared workspace
- Tool availability and configuration
- Access control and member management
