# Tasks

Tasks provide a way to automate interactions with the LLM through scripted chats. Tasks are made up of a series of steps that can be easily expressed through natural language. Tasks can also have **Arguments** that allow them to be called with inputs. For instance, search for "weather in New York" or "weather in London" can be the same task with different values for the argument `city`.

**Arguments** are optional and allow you to specify inputs to your task.

**Steps** represent instructions to be carried out by the task.

## Triggering Tasks

Tasks can be triggered in a variety of ways:

### On Demand

The default is On Demand, which means you can launch the task from the UI or through chat.

### Scheduled

You can trigger a task by scheduling it to run hourly, daily, weekly, or monthly along with a narrowed time window.

:::note
The timezone for scheduled tasks will be the timezone your Obot server is running in.
:::

