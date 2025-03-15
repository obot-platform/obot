# Agents

An agent is essentially a "base obot" that other obots can be created from.
Out of all the agents, one of them can be marked as default, and all non-admins will only be able to create
new obots based on that default agent. Admins can create obots from any agent.

Agents mostly have the same configuration parameters as an obot, but there are some differences to note.

### Capabilities

This section allows you to selectively enable or disable the following capabilities for all obots
created from the agent:
- Knowledge: files uploaded by the obot creator to serve as a knowledge set for the obot to search when needed
- Workspace Files: files uploaded by the obot creator and/or users to serve as a workspace for the obot to read and write to
- Database: this is the SQLite database that the obot can use to store data
- Tasks: if disabled, users will not be able to create [tasks](../20-concepts/06-tasks.md) on the obot
- Threads: if disabled, users will be limited to just one non-deletable thread with the obot

### Tools

This section allows you to configure the tools that will be available to all obots created from the agent.
Any tools left out of the agent will not be available in the obots created from it, so if you are configuring the
default agent, make sure you choose the full set of tools that the users will need.

Each tool can be configured in one of three modes:
- Always On: every obot created from the agent will have this tool, and it cannot be toggled off when chatting directly with the agent.
- Optional - On: obots created from the agent can choose to use this tool. When chatting with the agent (rather than an obot created from it), the tool will be available but can be toggled off.
- Optional - Off: obots created from the agent can choose to use this tool. When chatting with the agent (rather than an obot created from it), the tool will not be available by default but can be toggled on.

Tools can also be pre-authenticated at the agent level, much like they can at the obot level. This is generally not recommended,
unless you will be the only user of the agent.

### Advanced

#### Model

You can configure which model will be used when chatting with this agent and obots created from it.

#### Environment Variables

You can set key-value pairs of environment variables that will be available to all tools called by the agent and obots created from it.
