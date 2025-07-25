# Projects

A project is a collection that combines an LLM, a set of instructions, and MCP Servers to perform tasks, answer questions, and interact with its environment.

Below are the key concepts and fields you need to understand to build a project.

### Threads

Threads are different conversations with the project. They are useful for keeping context separated from different conversations. Some details like memories are shared between all threads in the project, but conversation history and uploaded files are not shared.

### Tasks

See [tasks](03-tasks.md).

### MCP Servers

MCP Servers provide tools, which dictate what a project can do and how it can interact with the rest of the world. The tools shipped with Obot help make their purpose clear. A few examples might include:

- Creating an email draft
- Sending a Slack message
- Getting the contents of a web page

Tools allow your project to perform actions and access data from the outside world.

### Memories

As you chat with your Obot project, you can ask it to remember things that are important. These memories will be added to the system prompt to ensure that the LLM takes them into account in all your conversations across threads. You can create a memory by asking the LLM to remember something. There are controls for viewing, editing, and removing memories in the 'Memories' tab on the left sidebar.

### Configuration

### Name and Description

These fields will be shown to users to help them identify and understand the project.

### Sytem Prompt

Instructions let you guide your chat project's behavior.
You should use this field to specify things like the project's objectives, goals, personality, and anything special it should know about its users or use cases.
Here is an example set of instructions for a HR Assistant designed to answer employee's HR-related questions and requests:

> You are an HR assistant for Acme Corporation. Employees of Acme Corporation will chat with you to get answers to HR related questions. If an employee seems unsatisfied with your answers, you can direct them to email `hr@acmecorp.com`.


### Built-In Capabilities

This dropdown controls whether certain capabilities are enabled or disabled. Currently this includes:

| Capability | Description | Enabled |
| -----------|-------------|---------|
| Memory | Allows the LLM to remember important information and share that across threads. | Yes |
| Knowledge | Use provided files to do RAG | Yes |
| Time | Provides information about the current date and the user's current time as well as time zone | Yes |

### Knowledge

The knowledge capability will let you supply your project with information unique to its use case.
You can upload files directly or pull in data from Notion, OneDrive, or a website.
If you've configured your project with knowledge, it will make queries to its knowledge database to help respond to the users' requests. The knowledge capability is enabled by default in new projects.

You should supply a useful **Knowledge Description** to help the agent determine when it should make a query.
Here is an example knowledge description for an HR Assistant that has documents regarding a companies HR policies and procedures:

> Detailed documentation about the human resource policies and procedures for Acme Corporation.

### Project Files

Project files are files that can be read and modified by any thread in a project. They are useful for prepopulating data for the llm and passing around information like state outside the scope of a specific thread.

#### Model Providers

Model providers can be configured to allow users to access different models in your project, and to configure the default model.

### Members

The members section allows you to manage invitations and users that can access the project. Users with access to the project have access to all threads, but they cannot modify the project configuration.

:::warning
Because all users with access to a project can access thread history, ensure you are not sharing sensitive information.