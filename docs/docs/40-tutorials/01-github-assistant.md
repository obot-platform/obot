# Create a Personal GitHub Assistant Project

This tutorial demonstrates how to create a project that interacts with GitHub through obot's **Chat Interface** and **MCP Gateway**. You'll build an AI assistant that can help you track GitHub issues, pull requests, and other development tasks.

This tutorial showcases:
- **Chat Interface**: Creating and configuring a project for conversational AI
- **MCP Gateway**: Connecting to GitHub through the Model Context Protocol
- **Integration**: How the chat interface and MCP gateway work together

:::note
As you configure the project, changes will be saved and applied automatically.
:::

## What You'll Learn

By the end of this tutorial, you'll understand how to:
1. Create and configure a project in the chat interface
2. Connect external services through MCP servers
3. Authenticate with GitHub using OAuth
4. Test and iterate on your AI assistant

## Prerequisites

- Access to an obot deployment (self-hosted or demo)
- A GitHub account with a personal access token
- Basic familiarity with GitHub concepts (issues, pull requests, repositories)

## 1. Creating Your Project

The **Chat Interface** is where you'll create and interact with your GitHub assistant.

### Access the Chat Interface
1. Go to your obot homepage
2. Click on your profile picture in the top right
3. Choose **Chat** from the dropdown menu

If you don't have an existing project, one will be created automatically. If you already have projects, click the **+** button in the left sidebar to create a new one.

### Configure Project Basics
1. **Name**: Set a descriptive name like "GitHub Assistant" or "My GitHub Helper"
2. **Description**: Write a brief description such as "Personal assistant for GitHub tasks and status updates"

### Define the System Prompt
Click the gear icon next to your project name in the sidebar to configure the project instructions. This system prompt defines your AI assistant's behavior and capabilities.

Try this example prompt:

```text
You are a smart assistant with access to the GitHub API through MCP tools.

Your main responsibilities:
- Help me track GitHub issues assigned to me
- Monitor pull requests where my review is requested
- Provide status updates on my GitHub activity
- Answer questions about repositories, commits, and GitHub workflows

When I ask for a "status update", provide:
1. All issues currently assigned to me
2. Pull requests where my review is requested  
3. Recent activity on repositories I care about

Be concise but informative, and always provide relevant links to GitHub.
```

## 2. Connecting GitHub via MCP Gateway

The **MCP Gateway** enables your project to securely connect with external services like GitHub.

### Add the GitHub MCP Server
1. In the left sidebar, click the **+** button next to `MCP Servers`
2. Search for "GitHub" in the server catalog
3. Select the GitHub MCP server from the results
4. Click **Connect To Server**

### Configure Authentication
1. You'll need a GitHub Personal Access Token:
   - Go to GitHub → Settings → Developer settings → Personal access tokens
   - Generate a token with appropriate permissions (repo, read:user, read:org)
   - Copy the token

2. In obot, paste your token in the `GitHub Personal Access Token` field
3. Click **Update** to save the configuration

The MCP Gateway will now handle secure communication between your project and GitHub's API.

## 3. Testing Your GitHub Assistant

Now that your project has access to GitHub through the MCP Gateway, test the integration:

### Basic Functionality Test
Start with a simple request to verify the connection:
```
Get the star count for the repository "torvalds/linux"
```

### Personal GitHub Data
Test with your own GitHub data:
```
Show me all repositories I own
```

### Status Update Feature
If you included the status update instructions, try:
```
Give me a status update on my GitHub activity
```

### Advanced Queries
Explore more complex interactions:
```
What are the most recent issues I've been assigned across all repositories?
```

## 4. Understanding the Integration

### How the Parts Work Together

1. **Chat Interface**: You interact with your GitHub assistant through natural conversation
2. **MCP Gateway**: Securely proxies requests to GitHub's API using the MCP protocol
3. **GitHub MCP Server**: Translates between MCP calls and GitHub API requests
4. **Your Project**: Uses the available tools to fulfill your requests

### Data Flow
```
You → Chat Interface → Project → MCP Gateway → GitHub MCP Server → GitHub API
```

The response flows back through the same path, with the MCP Gateway ensuring secure, authenticated access to your GitHub data.

## 5. Customization Ideas

Enhance your GitHub assistant by:

### Extended System Prompt
Add more specific instructions for your workflow:
- Repository priorities
- Notification preferences  
- Specific GitHub workflows you use

### Additional Context
Upload files with:
- Team member information
- Project priorities
- Coding standards or guidelines

### Tool Integration
Connect additional MCP servers for:
- Slack notifications about GitHub activity
- Email summaries of GitHub updates
- Integration with project management tools

## Best Practices

### Security
- Use personal access tokens with minimal required permissions
- Regularly rotate your GitHub tokens
- Don't share projects that contain sensitive repository access

### Effective Prompts
- Be specific about what information you want
- Use consistent terminology for commands like "status update"
- Provide context about which repositories matter most to you

### Project Organization
- Use clear, descriptive project names
- Document your project's purpose in the description
- Keep system prompts focused but comprehensive

## Troubleshooting

### Authentication Issues
- Verify your GitHub token has the correct permissions
- Check that the token hasn't expired
- Ensure the token is correctly entered in the MCP server configuration

### API Limitations
- GitHub API has rate limits; the MCP Gateway handles this automatically
- Some operations require specific permissions on repositories
- Private repositories require appropriate token permissions

This tutorial demonstrates how obot's three main components work together to create powerful, connected AI assistants that can interact with your development workflow through familiar conversational interfaces.
