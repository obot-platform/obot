# Create a Kubernetes Troubleshooting Workflow

## Overview

This guide demonstrates adding an automated AI workflow to an existing Kubernetes - PagerDuty monitoring setup.
A webhook event will be added to PagerDuty that triggers automated troubleshooting and remediation in Obot.
The automation is capable of downloading a runbook specified by the alert, searching the knowledge base for workflows, and then executing the diagnostic steps against the cluster.
The On-Call engineer will receive this information as notes in the PagerDuty incident.

![Workflow Overview](/img/webhook-overview.png)

## Prerequisites

- A Kubernetes cluster that is configured to send alerts to PagerDuty.
- A PagerDuty API Key.
- Kubeconfig file from the Kubernetes cluster you would like to interact with. (The workflow users read/write permissions to the cluster )
- Obot CLI installed and configured. See the [CLI installation instructions](/#getting-started)
- Obot server will need to be accessible from the internet.

## Set up the workflow

A workflow can be created in the Obot Admin UI, or it can be created using the Obot CLI. This example has several steps that lend it to be created via the CLI.

First, create a new file called `issue-triage.yaml` and add the following content:

<details>
    <summary>Complete <code>issue-triage.yaml</code></summary>

```yaml
type: workflow
Name: issue triage
Cache: false
Alias: issue-triage
Prompt: "You are a helpful assistant, your pagerduty email is found in the environment variable PAGERDUTY_EMAIL"
tools:
  - github.com/otto8-ai/experimental-tools/pagerduty-tool
  - github.com/otto8-ai/experimental-tools/kubectl
  - sys.http.html2text
Env:
  - name: PAGERDUTY_API_TOKEN
    description: Pagerduty API key
  - name: KUBECONFIG_FILE
    description: The full base64 encoded content of your kubeconfig file
  - name: PAGERDUTY_EMAIL
    description: A valid email address of a real user in PagerDuty
steps:
  - step: "Get the Incident ID from the webhook."
  - step: "Get the incident details"
  - step: "Acknowledge the incident"
  - step: “Get the PAGERDUTY_EMAIL env var. This is the user_email for all interactions with PagerDuty”
    tools:
    - sys.getenv
  - step: "Get the env value for ${OBOT_THREAD_ID}."
    tools: 
    - sys.getenv
  - step: "Add a note to the incident that Obot is looking into the issue, and a link to ${OBOT_SERVER_URL}/admin/thread/${OBOT_THREAD_ID}"
    tools: 
    - sys.getenv
  - step: "Get the incidents alerts"
  - if:
     condition: "Does the alert event contain an annotation called runbook_url"
     steps:
     - step: "Get the contents of the runbook_url, and determine which steps need to be taken"
     - step: "Follow the diagnosis steps using kubectl commands to troubleshoot the issue."
     - step: "If you can remediate by rolling back a deployment or rollout, do so"
     else:
     - step: "Query your knowledge set with the summary and description section of the alert. return the results of the tool call."
     - if:
        condition: "Did the previous step get diagnosis information."
        steps:
         - step: "Follow the diagnosis steps using kubectl commands to troubleshoot the issue."
        else:
         - step: "Get basic kubernetes information that would help troubleshoot this issue"
  - step: "Add a note to the incident with a bulleted list of the actions taken, the responses, and recommended next steps."
```

</details>

> *Note: If you want to use a read only user for the cluster, you should delete the step `If you can remediate by rolling back a deployment or rollout, do so`.*

Save the file and run the following command to create the workflow:

```bash
obot create issue-triage.yaml
```

You will see an ID returned as part of the output, you will need this value in the next steps.

## Authenticate the workflow

Let's prepare our data for the workflow to interact with Kubernetes and PagerDuty.

### Prepare the `kubeconfig` file

Your `kubeconfig` file needs to be base64 encoded, with the new lines removed.

```bash
cat ./kubeconfig | base64 | tr -d '\n' > kubeconfig.base64
```

You will also need your PagerDuty API key, and the email address of the user that Obot will use to interact with PagerDuty.

### Run the authentication command

```bash
obot workflow auth <ID>
```

Follow the prompts to authenticate. When asked for the `KUBECONFIG_FILE`, use the file notation `@kubeconfig.base64` to point directly to the file.

## Add Knowledge to the workflow

Visit the workflow in the Obot Admin UI. Click on `workflows > issue triage`.
Scroll to the bottom of the workflow form. Then click on the `+ Add Knowledge` button.
Select `Website` as the source.

Enter the following url: `https://runbooks.prometheus-operator.dev/`

Click `OK`.

This will take you to a form which will list the pages on the website. Select all of them by clicking the `+` icon for each line.

## Setting up Webhook Trigger

### Create the webhook in PagerDuty

On PagerDuty side, click integrations > Developer Tools > Generic Webhooks (v3)

Put the URL in. It should be `<OBOT_BASE URL>/api/webhooks/default/pd-hook`

Select the Scope type. In the demo setup, I had Scope Type = Service and Scope = Default Service

Deselect all events, and select `incident.triggered`

Click `Add Webhook` button.

You will get a "subscription created" pop up. Copy the secret so we can verify payloads.

### Create the webhook receiver in Obot

Go to the Obot Admin UI, click on the `Webhooks` tab, and click `Create Webhook`.

Name the webhook `pd-hook`, and use the secret you copied from PagerDuty.

Select the `issue-triage` workflow.

Fill in the secret from PagerDuty in the `secret` field.

In the validationHeader field, enter `X-PagerDuty-Signature`.

Click `Create Webhook`.
