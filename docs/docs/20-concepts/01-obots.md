# Obots

An obot is a program that combines AI, a set of instructions, and access to your services and data to perform tasks, answer questions, and interact with its environment.

Below are the key concepts and fields you need to understand to build an obot.

### Name and Description

These fields will be shown to the users of the obot to help them identify and understand the obot.

### Instructions

Instructions let you guide your obot's behavior.
You should use this field to specify things like the obot's objectives, goals, personality, and anything special it should know about its users or use cases.
Here is an example set of instructions for a HR Assistant designed to answer employee's HR-related questions and requests:

> You are an HR assistant for Acme Corporation. Employees of Acme Corporation will chat with you to get answers to HR related questions. If an employee seems unsatisfied with your answers, you can direct them to email hr@acmecorp.com.

### Tools

Tools dictate what an obot can do and how it can interact with the rest of the world. The tools shipped with Obot help make their purpose clear. A few examples include:
- Creating an email draft
- Sending a Slack message
- Getting the contents of a web page

Tools allow your obot to perform actions and access data from the outside world.

### Knowledge

Enabling the knowledge capability will let you supply your obot with information unique to its use case.
You can upload files directly or pull in data from Notion, OneDrive, or a website.
If you've configured your obot with knowledge, it will make queries to its knowledge database to help respond to the users' requests.

You should supply a useful **Knowledge Description** to help the agent determine when it should make a query.
Here is an example knowledge description for an HR Assistant that has documents regarding a companies HR policies and procedures:

> Detailed documentation about the human resource policies and procedures for Acme Corporation.

### Files

You can give an obot access to files that it can read and edit. The obot can also create new files for the user, and the user can upload files to it for the obot to work with.
The location that files are stored in is called the workspace. This works differently from knowledge, in that the user can interactively manage and edit these files when chatting with the obot.

### Tasks

See [tasks](06-tasks.md).

### Interface

#### Introduction

The introduction is a message that is displayed to the user before they begin to chat. It is a great place to set expectations for the user and let them know what they can do with the obot.

#### Starter Messages

Starter messages are a set of messages that are displayed to the user when they first start a chat with the obot.
They give the user a choice of pre-written messages that they can send to the LLM.
This is useful to help the user get started with the obot and to give them an idea of what they can do with it.
Even when starter messages are set, users will still be able to start with their own message if they want.

You can specify the model each agent will use. If you make no selection, the system default will be used. For more on models, see our [Models concept](models).

### Credentials

Most tools in Obot require some form of authentication, such as an API key or OAuth 2.0, to access the services they interact with.
Under most circumstances, the user of an obot will be prompted for an API or asked to sign in when needed.
However, it is possible to pre-authenticate the tools on an obot by providing the necessary credentials in the obot configuration.

:::warning
If you share an obot with pre-authenticated credentials, you are also sharing the credentials with all users of the obot.
They will be able to access the same services and data as the obot.
This is generally not recommended.
:::

### Share

By default, each obot is private, accessible only to the user who created it. However, you can enable sharing,
which gives you a link that you can send to others. Any users with the link will be able to use the obot as well.
