# Configure the GitHub CI Failure Notifier Obot

The **GitHub CI Failure Notifier Obot** is an obot that sends a message to a Slack workspace when a GitHub Actions job fails.

## Prerequisites

- A GitHub repository that runs GitHub Actions jobs  
- A Slack workspace with the Obot Slack integration installed  

## Configuration

Create a copy of the featured **GitHub CI Failure Notifier** obot and open it up.

### 1. Create a Task on the Obot

In the **Obot Editor**, open the obot you are configuring, and in the **Tasks** section on the left sidebar, click the `+` icon to add a new task.  

- Set a **Name** and **Description** for the task.  

Then, configure the following four steps within the task:

```text
1. If the action field is not "completed", call the abort task tool. Otherwise, say "pipeline completed".

2. Look at the steps. If none of the steps have status "failure", call the abort task tool. Otherwise, say "at least one step failed".

3. Examine the logs by calling the tool to get them, with the proper owner, repo, and job ID. Do some analysis and see if you can figure out what went wrong.

4. Send a Slack message to [person name or channel name] with details about the name of the repo, the job, the failed step(s), and the HTML URL in your message. Use plaintext (not Markdown). Also include your analysis about what went wrong.
```

> **Important:** Be sure to replace `[person name or channel name]` with the actual Slack channel or user where you want the message sent.

Set the **Trigger** to `On Webhook`. This will generate a webhook URL that you will use in the next step.

### 2. Configure the GitHub Webhook

In your GitHub repository:

- Go to **Settings** -> **Webhooks**.
- Click **Add webhook**.

Fill out the following fields:

- **Payload URL**: Paste the webhook URL you got from the previous step.
- **Content type**: `application/json`
- **Secret**: (leave blank)
- **Which events would you like to trigger this webhook?**: Choose **Let me select individual events**.
  - Check **Workflow jobs**.
  - Make sure to uncheck **Pushes**.

Click **Add webhook** to save it.

### 3. Test and Authenticate

To test the webhook:

- Manually trigger a GitHub Actions job in your repository.

The obot task should automatically start running. You may see some task runs that abort early if the job is not yet completed.

Once the job finishes, the final task will:

- Process the job results.
- Attempt to retrieve logs of the failed job from GitHub.
- Prompt you to authenticate with GitHub on the first run.
- Attempt to send a Slack message with failure details.
- Prompt you to authenticate with Slack on the first run.

> ⚙️ After initial authentication, future runs will proceed without needing to authenticate again.
