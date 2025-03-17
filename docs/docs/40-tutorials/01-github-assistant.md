# Create a Personal GitHub Task Obot

This is a short tutorial demonstrating how to create an obot that interacts with GitHub. The obot will help you keep track of the work assigned to you in GitHub.

:::note
As you configure the obot, changes will be saved and applied automatically.
:::

## 1. Setting up the obot

Start by going to the main obot.ai page and scroll down until you see **+ Create New Obot**. Click the button to create a new obot.
Set the obot name and description to whatever you would like in the fields on the left hand side.

Next, write some instructions for the obot.
This is a prompt that explains what you would like the obot to do for you.
Here is one example you can try:

```text
You are a smart assistant with access to the GitHub API.
Please answer my questions related to GitHub.
When I ask for a "status update", list all of the issues assigned to me, as well as pull requests where my review is requested.
```

## 2. Adding the tools

Now we need to give the obot access to the GitHub tools.
Click to expand the **Tools** section
Click the **+ Tools** button and add `GitHub`.
Click on the `+` button at the right side of the category name to add all the GitHub tools to the obot.

## 3. Testing the obot

You can now begin chatting with the obot in the chat interface to the right.
Start by asking it to do something simple, like getting the star count of the repo "torvalds/linux".
When it makes its first tool call, you will have to sign in to GitHub to authorize the obot to access your account.
Then, try having it interact with things specific to your account.
If you gave your obot instructions about a "status update", ask for one and see what it does.

## 4. Sharing the obot (optional)

If you're happy with the obot and want other users to try out your obot,
you can click to expand the **Share** section when editing the obot configuration.
Here you can click share and copy the link to share with others.
