import { z } from "zod";

import { assetUrl } from "~/lib/utils";

import { OAuthAppSpec, OAuthFormStep, getOAuthLinks } from "../oauth-helpers";

const schema = z.object({
    clientID: z.string().min(1, "Client ID is required"),
    clientSecret: z.string().min(1, "Client Secret is required"),
});

const scopes = [
    "channels:history",
    "groups:history",
    "im:history",
    "mpim:history",
    "channels:read",
    "files:read",
    "im:read",
    "search:read",
    "team:read",
    "users:read",
    "groups:read",
    "chat:write",
    "groups:write",
    "mpim:write",
    "im:write",
];

const steps: OAuthFormStep<typeof schema.shape>[] = [
    {
        type: "markdown",
        text:
            "All steps will be performed on the [Slack API Dashboard](https://api.slack.com/apps).\n\n" +
            "### Step 1: Create a Slack App\n" +
            "If you've already created a Slack app, you can skip this step.\n",
    },

    {
        type: "sectionGroup",
        sections: [
            {
                title: "How do I create a Slack App?",
                steps: [
                    {
                        type: "markdown",
                        text:
                            "- From the [Slack API Dashboard](https://api.slack.com/apps), create a new app and select `From scratch`\n" +
                            "- Give your app a `Name` and select a `Workspace`\n" +
                            "- Click `Create`\n",
                    },
                ],
            },
        ],
    },
    {
        type: "markdown",
        text:
            "### Step 2: Add the Redirect URL\n" +
            "- From the [Slack API Dashboard](https://api.slack.com/apps), click on your app and select `OAuth & Permissions`\n" +
            "- In the `Redirect URLs` section, click `Add New Redirect URL`\n" +
            "- Add the following URL: ",
    },
    {
        type: "copy",
        text: getOAuthLinks("slack").redirectURL,
    },
    {
        type: "markdown",
        text: "- Click `Save URLs` to save the changes.\n",
    },
    {
        type: "markdown",
        text:
            "### Step 3: Add Scopes\n" +
            "- Navigate to the `OAuth & Permissions` tab from the sidebar.\n" +
            "- Locate the `User Token Scopes` section and add the following scopes:\n",
    },
    {
        type: "sectionGroup",
        sections: [
            {
                title: "Scopes: ",
                displayStepsInline: true,
                defaultOpen: true,
                steps: scopes.map(
                    (scope) =>
                        ({
                            type: "copy",
                            text: scope,
                        }) as OAuthFormStep<typeof schema.shape>
                ),
            },
        ],
    },
    {
        type: "markdown",
        text:
            "### Step 4: Install the App\n" +
            "- Navigate to the `OAuth & Permissions` tab from the sidebar.\n" +
            "- Click on the `Install App to Workspace` (or `Reinstall to <App Name>` if it's already installed) button.\n",
    },
    {
        type: "markdown",
        text:
            "### Step 5: Register OAuth App in Otto\n" +
            "Click the `Basic Information` section in the side nav, locate the `Client ID` and `Client Secret` fields, copy/paste them into the form below, and click `Submit`.\n",
    },
    { type: "input", input: "clientID", label: "Client ID" },
    {
        type: "input",
        input: "clientSecret",
        label: "Client Secret",
        inputType: "password",
    },
];

const disableConfiguration = !getOAuthLinks("slack")
    .redirectURL.toLowerCase()
    .startsWith("https");
export const SlackOAuthApp = {
    schema,
    refName: "slack",
    type: "slack",
    displayName: "Slack",
    logo: assetUrl("/assets/slack_logo_light.png"),
    darkLogo: assetUrl("/assets/slack_logo_dark.png"),
    steps,
    disableConfiguration,
    disabledReason: disableConfiguration
        ? "Slack requires that redirect URLs start with `https`. Since this application is running on `http`, you will need to redeploy Otto using `https` in order to configure a custom Slack OAuth app."
        : undefined,
} satisfies OAuthAppSpec;
