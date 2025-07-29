# Chat Interface Configuration

The Chat Configuration page in the Admin sets global defaults and system-wide settings that affect all projects and users in your obot deployment. These configurations provide baseline settings that individual projects can inherit and customize.

## Global Defaults

- **Name**: The default name to be used for projects
- **Icon**: Select the image to be used as the default avatar for projects
- **Description**: The default description for new projects
- **Introductions**: The introductory message that will show up at the beginning of new conversations
- **Instructions**: The default instructions that are sent to the LLM that define capabilities, tone, behavior, etc.
- **Allowed Models**: Available models that projects can choose from
- **Default Model**: Which LLM to use by default for new projects

## Project Scope Configuration

As the owner of a project, you can configure most of the same fields in the project-scoped configuration, with a couple notable differences.

- There is no model provider configuration at the project level.
- Upload files to be used with 'Knowledge' (RAG) in your project.
- Add and configure MCP Servers for your project
- Add, modify, and remove Memories in your project
- Add, modify, run, and remove Tasks associated with your project
