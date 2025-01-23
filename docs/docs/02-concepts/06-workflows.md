# Workflows

A workflow is a series of steps that can be easily expressed through natural language to achieve a task or process. Workflows have the same fields as [agents](agents) with the addition of **Parameters** and **Steps**.

**Parameters** are optional and allow you to specify inputs to your workflow. This is particularly useful when another workflow or an agent is calling your workflow.

**Steps** represent instructions to be carried out by the workflow. A step can have it's own set of tools and can even call out to other workflows or agents. Obot supports two special types of steps: **If Statements** and **While Loops**.

**If Statements** allow you to specify a condition and different actions to take based on whether that condition is true or false.

**While Loops** allow you to specify a condition and set of steps. As long as the condition evaluates to true, the steps will be continuously executed in a loop.

### Triggering Workflows

#### CLI

You can trigger a workflow in a few ways. The first is via the **invoke** cli command. Here's an example that invokes a workflow that has two parameters:

```
obot --debug invoke w1km9xw "name='John Doe', address='123 Main Street'"
```

You can find the workflow id by listing workflows:

```
obot workflows
```

#### Scheduled

You can trigger a workflow by scheduling it to run hourly, daily, weekly, or monthly.

#### Webhook

1. Go to the Workflow Trigger page in the Obot UI.
2. Click **Create Trigger**.
3. Select **Webhook** as the trigger type.
4. Fill in the required fields:
   - **Name**: The name of the webhook trigger.
   - **Workflow**: The workflow to invoke.

In addition to the above fields, there are several optional fields, described below.

**Headers** can be specified as an array. You can add headers like `X-HEADER-1` and `X-HEADER-2`.

If any of these headers are seen in the webhook request, they'll be passed to the workflow as well.

**Secret** and **validationHeader** can be used to secure webhook invocations.

Services that offer webhook integration typically supply a shared secret used to compute a signature for the request and expect the webhook receiver to verify the signature, which Obot does.
Two such services are GitHub and PagerDuty. To understand how to set these fields, you can find their webhook documentation here:

- https://docs.github.com/en/webhooks/using-webhooks/validating-webhook-deliveries
- https://developer.pagerduty.com/docs/28e906a0e4f36-verifying-signatures

Refer to your service's webhook documentation to find the values to set for these fields.

#### Email

:::note
This will require configuration as described in the [Email Webhook Configuration](/configuration/email-webhook#configure-obot) documentation.
:::

You can trigger a workflow by sending an email to an email address configured in Obot. The email address should be in the format of `{name}@{email_server_name}`.
The `{email_server_name}` should be configured from [here](/configuration/email-webhook#configure-obot).

To create an email trigger

1. Go to the Workflow Trigger page in the Obot UI.
2. Click **Create Trigger**.
3. Select **Email** as the trigger type.
4. Fill in the required fields:
   - **Name**: The name of the email trigger.
   - **Alias**: The alias name will match the name of the email address that you want to receive emails to. For example, if the recipient email address is john@{email_server_name}, the alias value should be set as john. If you leave this field blank, alias will be generated.
   - **Workflow**: The workflow to invoke.
5. Click **Create**.

Once this is created, emails sent to the email address will trigger the workflow. The following data will be passed to the workflow:

- `from`: The email address of the sender.
- `to`: The email address of the receiver.
- `subject`: The subject of the email.
- `body`: The body of the email.

You can use these data in your workflow to perform different actions.
