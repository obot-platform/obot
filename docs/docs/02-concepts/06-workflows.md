# Workflows

A workflow is a series of steps that can be easily expressed through natural language to achieve a task or process. Workflows have the same fields as [agents](agents) with the addition of **Parameters** and **Steps**.

**Parameters** are optional and allow you to specify inputs to your workflow. This is particularly useful when another workflow or an agent is calling your workflow.

**Steps** represent instructions to be carried out by the workflow. A step can have it's own set of tools and can even call out to other workflows or agents. Acorn supports two special types of steps: **If Statements** and **While Loops**.

**If Statements** allow you to specify a condition and different actions to take based on whether that condition is true or false.

**While Loops** allow you to specify a condition and set of steps. As long as the condition evaluates to true, the steps will be continuously executed in a loop. 

### Triggering Workflows

You can trigger a workflow in a few ways. The firest is via the **invoke** cli command. Here's an example that invokes a workflow that has two parameters:
```
acorn --debug invoke w1km9xw "name='John Doe', address='123 Main Street'"
```
You can find the workflow id by listing workflows:
```
acorn workflows
```

Another way to trigger a workflow is via a **webhook**. This feature is still under development and doesn't yet have a corresponding CLI commmand, but you can create a webhook via **curl**. Here's an example:
```
curl -X POST 'http://127.0.0.1:8080/api/webhooks' -d '{ \
  "description": "Webhook to respond to pagerduty events", \
  "alias": "pd-hook", \
  "workflowID": "w1km9xw"}'
```
This will produce a webhook that can be called at http://localhost:8080/api/webhooks/pd-hook. When called, the body of the webhook request will be sent to the workflow as input.

In addition to the above fields, there are several optional fields, described below.

**Headers** can be specified as an array. You can add this field to the above curl command like this:
```
{ ... "headers" ["X-HEADER-1", "X-HEADER-2"] ... }
```
If any of these headers are seen in the webhook request, they'll be passed to the workflow as well.

**Secret** and **validationHeader** can be used to secure webhook invocations. You can add these fields like this:
```
{ ...
  "secret": "<SHARED SECRET>",
  "validationHeader": "<SIGNATURE HEADER>"
  ...
}
```

Services that offer webhook integration typically supply a shared secret used to compute a signature for the request and expect the webhook receiver to verify the signature, which Acorn does. Two such services are GitHub and PagerDuty. To understand how to set these fields, you can find their webhook documentation here:

- https://docs.github.com/en/webhooks/using-webhooks/validating-webhook-deliveries
- https://developer.pagerduty.com/docs/28e906a0e4f36-verifying-signatures

Refer to your service's webhook documentation to find the values to set for these fields.


