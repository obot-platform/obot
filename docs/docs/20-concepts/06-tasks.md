# Tasks

Tasks provide a way to automate interactions with the LLM through scripted chats. Tasks are made up of a series of steps that can be easily expressed through natural language. Tasks can also have **Parameters** that allow them to be called with inputs. For instance, search for "weather in New York" or "weather in London" can be the same task with different values for the parameter `city`.

**Parameters** are optional and allow you to specify inputs to your task.

**Steps** represent instructions to be carried out by the task. A step can have it's own set of tools and can even call out to other tasks or agents.

## Triggering Tasks

Tasks can be triggered in a variety of ways:

### On Demand

The default is On Demand, which means you can launch the task from the UI or through chat.

### Scheduled

You can trigger a task by scheduling it to run hourly, daily, weekly, or monthly along with a narrowed time window.

### Webhook

1. Within your obot instance, click "Edit" for an existing task or create a new task.
1. Click the dropdown that says **On Demand** and select **Webhook**.
1. You will then see a URL you can use to trigger the task. Copy and paste that into your webhook provider.

You can also provide a webhook body to use while testing the task during development.

### Email

You can trigger a task by sending an email to the task.

To create an email trigger

1. Within your obot instance, click "Edit" for an existing task or create a new task.
1. Click the dropdown that says **On Demand** and select **On Email**.
1. You will be provided with an email address that you can use to trigger the task.

You will also be able to provide a sample email to use while testing the task during development.

Once this is created, emails sent to the email address will trigger the task. The following data will be passed to the task:

- `from`: The email address of the sender.
- `to`: The email address of the receiver.
- `subject`: The subject of the email.
- `body`: The body of the email.

You can use these data in your task to perform different actions.
