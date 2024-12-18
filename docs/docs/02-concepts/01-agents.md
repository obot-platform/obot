# Agents

An agent is a program that combines AI, a set of instructions, and access to your services and data to perform tasks, answer questions, and interact with its environment.
Administrators create agents for end-users to interact with. Below are the key concepts and fields you need to understand to build an agent.

### Name and Description
These fields will be shown to your end-users to help them identify and understand your agents.

### Instructions
Instructions let you guide your agent's behavior.
You should use this field to specify things like the agent's objectives, goals, personality, and anything special it should know about its users or use cases.
Here is an example set of instructions for a HR Assistant designed to answer employee's HR-related questions and requests:

> You are an HR assistant for Acme Corporation. Employees of Acme Corporation will chat with you to get answers to HR related questions. If an employee seems unsatisfied with your answers, you can direct them to email hr@acmecorp.com. 

### Model
You can specify the model each agent will use. If you make no selection, the system default will be used. For more on models, see our [Models concept](models).

### Tools
Tools dictate what an agent can do and how it can interact with the rest of the world. The tools shipped with Obot help make their purpose clear. A few examples include:
- Creating an email draft
- Sending a Slack message
- Getting the contents of a web page

You can configure an agent with **Agent Tools** and **User Tools**, described below:
- **Agent Tools** will always be enabled on an agent. The agent will always be able to call them.
- **User Tools** are optional. They are available for a user to add to their chat with the agent. Users can add and remove these tools at will.

### Knowledge
Knowledge lets you supply your agent with information unique to its use case.
You can upload files directly or pull in data from Notion, OneDrive, or a website.
If you've configured your agent with knowledge, it will make queries to its knowledge database to help respond to the users' requests.

You should supply a useful **Knowledge Description** to help the agent determine when it should make a query.
Here is an example knowledge description for an HR Assistant that has documents regarding a companies HR policies and procedures:

> Detailed documentation about the human resource policies and procedures for Acme Corporation.

### Preview Chat and Threads
When creating an agent, you can chat with it to see how it behaves based on how you've configured it.
You can even create access past preview threads to compare behavior as your iterate on it.


### Publishing
When you publish an agent, you make it available for end-users to chat with.
You'll first be asked to specify an alias for the agent. This alias will be used to generate a unique URL for the agent.
Only alphanumeric characters and hyphens are allowed.
Once you've picked an alias and hit the publish button, the agent will immediately be available for users to discover and chat with.

The end-user chat interface is available at the root of the domain on which Obot is running.
For example, if you accessed the admin UI at `https://obot.acmecorp.com/admin`, the end-user chat UI is available at `https://obot.acmecorp.com/`.
A published agent will be available in the agent dropdown on this page as well as at its own dedicated page based on its alias.
For example, if you gave your agent the alias `hr-assistant`, it would be available at https://obot.acmecorp.com/hr-assistant`.

It is possible to request the same alias for two agents.
This is to allow you to seamlessly swap agents out without your users noticing.
For example, suppose you've published your first agent with the alias `hr-assistant` and it's available at `https://obot.acmecorp.com/hr-assistant`.
If you want to experiment with a new version of your agent, you can create a brand new one, refine its behavior, and when you're ready, set its alias to `hr-assistant` as well.
When you're ready to switch your users to the new agent, just unpublish your first agent.
Users will be swapped over to the new agent without losing any of their chat history.
