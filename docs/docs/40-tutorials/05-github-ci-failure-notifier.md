# Configure the GitHub CI Failure Notifier Obot

The **GitHub CI Failure Notifier Obot** is an obot that sends a message to a Slack workspace when a GitHub Actions job fails.

## Prerequisites

- A GitHub repository that runs GitHub Actions jobs  
- A Slack workspace with the Obot Slack integration installed  

## Configuration

### 1. Configure the Task

Open up the featured GitHub CI Failure Notifier obot, and edit the existing task called `Webhook`.
Set the trigger to `On Webhook`. This will generate a webhook URL that you will use in the next step.

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
