# Create a Slack Alerts Obot

This is a short tutorial demonstrating how to create an **Obot** that helps with alerts in a Slack channel.

> **Note:**
> As you configure the obot, changes are saved and applied automatically — no need to click "Save".

## 1. Setting up the Obot

Start by visiting [Obot](https://obot.ai) scroll down and click **+ New Obot** to begin creating one.  

In the **General** section on the left side:  

- Set a **Name** and **Description** for the obot (e.g., "Slack Alerts Helper").  
- Write **Instructions** to tell the obot what you want it to do. Be specific about which Slack channel contains the alerts, and what systems or types of alerts to expect.  

```text
You are a smart assistant with expertise in Kubernetes and access to the Slack API.  
Please help me with my alerts in Slack. They are in the channel #alerts.  
```

Here’s what the editor looks like when starting a new obot:  

## 2. Adding Slack Tools

To give the obot access to Slack, scroll down to the **Tools** section in the left sidebar.  

- Click **+ Add Tool**.  
- Search for `Slack`.  
- You’ll see a category of Slack tools appear — click the toggle on the right side of that category to add all Slack-related tools to your obot.  

This allows the obot to read alerts from Slack channels and respond appropriately.  

## 3. Testing the Obot

Once configured, you can test the obot using the **chat interface** on the right side of the editor.  

Try asking it something like:  

```text
What alerts have fired today?
Which alerts have fired more than once?
```

> 💡 **Note:** The first time the obot tries to interact with Slack, you’ll be prompted to log in and authorize access to your Slack account.  

You can also ask the obot to suggest remediation steps for specific alerts.  

## 4. Sharing the Obot (Optional)

If you’re satisfied with how the obot works and want others to use it:  

- Use the **Share** section in the left sidebar to manage access.  
- Click **Share** and copy the link to share with others.
