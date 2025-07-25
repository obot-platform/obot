# Create a Slack Alerts Project

This is a short tutorial demonstrating how to create an Obot project that helps with alerts in a Slack channel.

:::note
As you configure the project, changes will be saved and applied automatically.
:::

## 1. Setting up the project
Start by going to the Obot homepage. Click on your profile picture in the top right and chose **Chat** from the dropdown.
If you do not have an existing project, one will automatically be created for you. If you do already have a project, you can click on the **+** in the left sidebar next to the name of the project you are currently in.
Set the project name and description to whatever you would like in the fields on the left hand side.

Next, name your project and optionally write a description of what the project should be used for.

Next, click the gear nex to the project name in the sidebar and write some instructions for the project.
This is a prompt that explains what you would like the project to do for you.
Here is one example you can try:

```text
You are a smart assistant with expertise in Kubernetes and access to the Slack API.  
Please help me with my alerts in Slack. They are in the channel #alerts.  
```

## 2. Adding Slack Tools

Now we need to give the project access to the Slack Tools.
Click the **+** button next to the `MCP Servers` header in the left sidebar.
Type `Slack` into the search box, and select the GitHub MCP server.
Click the **Connect To Server** button.
Fill our the required `Slack Bot Token` and `Slack Team ID` fields, then click **Update** to save the configuration.

This allows the project to read alerts from Slack channels and respond appropriately.

## 3. Testing the project

Once configured, you can test the project using the **chat interface** on the right side of the editor.

Try asking it something like:

```text
What alerts have fired today?
Which alerts have fired more than once?
```

You can also ask the chat to suggest remediation steps for specific alerts.