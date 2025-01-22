# Integrating Tools with OAuth

One of the most powerful features of Obot is its ability to integrate agents with your services using custom tools and OAuth apps.
This guide will walk you through the process of creating and using these. We'll be integrating with [GitLab](https://about.gitlab.com/).

The high-level steps we'll follow are:
1. Create our custom tool
2. Create our OAuth app
3. Configure a credential tool that integrates our custom tool and OAuth app
4. Register the tool
5. Use the tool in an agent

### Prerequisites
You'll need a [GitLab](https://gitlab.com/) account with at least one project (their equivalent of a GitHub repo).

### Create our custom tool
The source for the tool we are creating can be found at https://github.com/otto8-ai/gitlab-example-tool.
This guide won't cover writing the Python code for the tool, so feel free to clone or fork this repo.
We will review the **tool.gpt** file:

```
Name: List Projects
Description: List the user's GitLab Projects
Credential: ./credential

#!/usr/bin/env python3 ${GPTSCRIPT_TOOL_DIR}/projects.py

---
!metadata:*:category
GitLab

---
!metadata:*:icon
https://cdn.jsdelivr.net/npm/@phosphor-icons/core@2/assets/duotone/gitlab-logo-duotone.svg

--
!metadata:*:oauth
gitlab-example
```

This tool.gpt file just has a single tool name "List Projects" defined.
You can define more tools by separating them with `---`.
You can see that this tool defines a name, description, credential (we'll revisit this in a later step), and a command that is the actual code to be run.
If you review the repository, you'll notice that `projects.py` is one of the files in the repository.

There are three metadata sections: one for category, one for icon and one for oauth. These will be used to display the tool in the Obot UI and tie oauth integration with the GitLab Oauth credentials we'll set up in the next step.

The `oauth` metadata section is important. It tells Obot that this tool uses the `gitlab-example` OAuth integration. It should match with the `integration` field in the credential tool we'll create in the next step.

### Configure a credential tool that integrates our custom tool and OAuth app
The credential tool can be found in our example repo at https://github.com/otto8-ai/gitlab-example-tool/blob/main/credential/tool.gpt. Here's the contents:

```
Name: GitLab OAuth Credential
Share Credential: github.com/obot-platform/tools/oauth2/tool.gpt as gitlab-example
    with GITLAB_OAUTH_TOKEN as env and
        gitlab-example as integration and
        "read_api read_user" as scope
Type: credential
```

Here is a breakdown of the above:
- `Name` is not too important. It can be whatever name you choose
- `Share Credential: github.com/obot-platform/tools/oauth2/tool.gpt as gitlab-example` causes this tool to use Obot's OAuth framework.
- `gitlab-example as integration` ties this tool to the OAuth app we created because it matches the value set for the **integration** field.
- `"read_api read_user" as scope` indicates the scopes that will be requested. They need to be the same as or a subset of the scopes granted when creating the OAuth application in GitLab.
- `Type: credential` tells Obot that this is a credential tool

Returning to our custom tool, we use this credential tool via this line:

```
Credential: ./credential
```

That is a relative path reference to the credential directory where the credential tool is defined.

Once you've finished with all this, the tool must be pushed to a GitHub repository.
Again, the original version of this tool is at https://github.com/otto8-ai/gitlab-example-tool.
You can use this value directly if you did not choose to fork the repo.

### Register the tool
Next we need to register the tool in your Obot installation. Go to the Tools page and click **Register New Tool**.
Then, drop in the GitHub repo where your tool lives. If you're using ours, you would drop in **github.com/otto8-ai/gitlab-example-tool**.
After a few moments of processing, you should see a new tool called "List Projects" at the top of the page in your Custom Tools section.

### Create our OAuth App
Next we need to create our OAuth app in Obot. This always involves also creating an equivalent resource in the service provider (GitLab in this case).
Each service differs in how you do this. For GitLab, the guide is [here](https://docs.gitlab.com/ee/integration/oauth_provider.html).
For this guide, follow the [Create a User Owned Application](https://docs.gitlab.com/ee/integration/oauth_provider.html#create-a-user-owned-application) instructions.

Once you get to the step where you're asked to supply a Redirect URL, go to your Obot installation, navigate to Tools, and find the "List Projects" tool you created above. Click the cogwheel on the top-left of the tool and select **Configure OAuth**.
You're then asked to supply a name, integration (which will be auto-filled), authorization URL, and token URL. Set the name to "GitLab Example".

GitLab's documentation doesn't make this clear, but the authorization and token URLs are as follows:

- Authorization: https://gitlab.com/oauth/authorize
- Token: https://gitlab.com/oauth/token

Set these values accordingly and click Next. You'll now be presented with a Redirect URL and be asked to supply a Client ID and Client Secret.
First, return to GitLab, supply the Redirect URL, select the scopes **read_api** and **read_user**, and click **Save application**.
You'll then be presented with the Client ID (which they call Application ID) and Client Secret (which they just call Secret).
Return to Obot, enter these values, and click Submit. Your OAuth has been set up and the tool is now ready for use.

### Use the tool in an agent
Now, we can use the tool in an agent. Create a new agent, click **Add Tool** under the Agent Tools section, find your GitLab tool, and add it.
After that, just ask "what are my gitlab projects?" in the chat. You should see the List Projects tool begin to get invoked and then be prompted to authenticate with GitLab.
Authenticate with GitLab, and you'll see the tool call complete successfully and your projects listed.

That concludes our guide. Use this as a jumping off point to create your own integrations.
