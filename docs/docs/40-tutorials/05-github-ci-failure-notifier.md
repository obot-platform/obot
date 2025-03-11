# Configure the GitHub CI Failure Notifier Obot

The GitHub CI Failure Notifier Obot is an obot that sends a message to a Slack workspace when a GitHub Actions job fails.

## Prerequisites

- A GitHub repo that runs GitHub Actions jobs
- A Slack workspace with the Obot Slack integration installed

## Configuration

### 1. Create a task on the obot

In the Obot UI, open the side panel for your obot and click the `+` icon next to "Tasks".
Set a name and description for the task.

Then, configure the following four steps on the task:

```
1. If the action field is not "completed", call the abort task tool. Otherwise, say "pipeline completed".

2. Look at the steps. If none of the steps have status "failure", call the abort task tool. Otherwise, say "at least one step failed".

3. Examine the logs by calling the tool to get them, with the proper owner, repo, and job ID. Do some analysis and see if you can figure out what went wrong.

4. Send a Slack message to [person name or channel name] with details about the name of the repo, the job, the failed step(s), and the HTML URL in your message. Use plaintext (not Markdown). Also include your analysis about what went wrong.
```

:::important
Be sure to set the `[person name or channel name]` to the name of the Slack channel or user that you want to send the message to.
:::

Select "On Webhook" for the trigger. This will display the webhook URL that we will need for the next step.

### 2. Configure the GitHub webhook

In your GitHub repo, go to Settings -> Webhooks. Click on the "Add webhook" button. Configure the following things on the webhook:

- Payload URL: the webhook URL that we got from the previous step.
- Content type: application/json
- Secret: leave blank
- Which events would you like to trigger this webhook?: Let me select individual events.
    - Select the following events:
        - Workflow jobs
    - Also be sure to uncheck "Pushes"

Click the "Add webhook" button.

### 3. Test and authenticate

To test the webhook, manually trigger a job in your repo. The obot tasks should start running. There will be a few task runs that will abort because the job is not yet complete.
The last task will continue to run and process the job results. It will attempt to get the logs of the failed job from GitHub.
When this happens, you will be prompted to authenticate with GitHub. It will then attempt to send a message to Slack, and you will be prompted to authenticate with Slack.
On future task runs, you will not be prompted to authenticate again.
