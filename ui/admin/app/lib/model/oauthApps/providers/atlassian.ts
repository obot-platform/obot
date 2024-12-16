import { z } from "zod";

import {
    OAuthAppSpec,
    OAuthFormStep,
    getOAuthLinks,
} from "~/lib/model/oauthApps/oauth-helpers";
import { assetUrl } from "~/lib/utils";

const schema = z.object({
    clientID: z.string().min(1, "Client ID is required"),
    clientSecret: z.string().min(1, "Client Secret is required"),
});

const steps: OAuthFormStep<typeof schema.shape>[] = [
    {
        type: "markdown",
        text:
            "### Step 1: Create a new Atlassian OAuth 2.0 Integration\n" +
            "- Navigate to [Create a new OAuth 2.0 (3LO) integration](https://developer.atlassian.com/console/myapps/create-3lo-app)\n" +
            "- Enter `Obot` as the integration name.\n" +
            "- Click the checkbox to the terms and conditions.\n" +
            "- Click the `Create` button.\n",
    },
    {
        type: "markdown",
        text:
            "### Step 2: Configure OAuth Scopes\n" +
            "Configure required OAuth Scopes by completing both sections below.\n",
    },
    {
        type: "sectionGroup",
        sections: [
            {
                title: "User identity API Scopes",
                steps: [
                    {
                        type: "markdown",
                        text:
                            "- Navigate to the `Permissions` tab in the sidebar.\n" +
                            "- Click on the `Add` button for `User identity API`\n" +
                            "- Click on the `Configure` button for `User identity API`\n" +
                            "- Click on the `Edit Scopes` button to open the `Edit User identity API` modal.\n" +
                            "- Click the checkboxes to select the `read:me` and `read:account` scopes.\n" +
                            "- Click on the `Save` button.\n",
                    },
                ],
            },
            {
                title: "Jira API Scopes",
                steps: [
                    {
                        type: "markdown",
                        text:
                            "- Navigate to the `Permissions` tab in the sidebar.\n" +
                            "- Click on the `Add` button for `Jira API`\n" +
                            "- Click on the `Configure` button for `Jira API`\n" +
                            "- Click on the `Edit Scopes` button to open the `Edit Jira API` modal.\n" +
                            "- Click the checkboxes to select the `read:jira-work`, `write:jira-work`, and `read:jira-user` scopes.\n" +
                            "- Click on the `Save` button.\n",
                    },
                ],
            },
        ],
    },
    {
        type: "markdown",
        text:
            "### Step 3: Configure your OAuth Consent Screen\n" +
            "- Navigate to the `Authorization` tab in the sidebar.\n" +
            "- Click on the `Add` button for `OAuth 2.0 (3LO)`.\n" +
            "- Enter the URL below in the `Callback URL` box and click on the `Save changes` button:\n",
    },
    {
        type: "copy",
        text: getOAuthLinks("atlassian").redirectURL,
    },
    {
        type: "markdown",
        text:
            "### Step 4: Register your OAuth App credentials with Obot\n" +
            "- Navigate to the `Settings` tab in the sidebar.\n" +
            "- Enter the `Client ID` and `Client Secret` from the `Authentication details` section into the fields below\n",
    },
    { type: "input", input: "clientID", label: "Client ID" },
    {
        type: "input",
        input: "clientSecret",
        label: "Client Secret",
        inputType: "password",
    },
];

export const AtlassianOAuthApp = {
    schema,
    alias: "atlassian",
    type: "atlassian",
    displayName: "Atlassian",
    logo: assetUrl("/assets/atlassian_logo.svg"),
    steps: steps,
    noGatewayIntegration: true,
} satisfies OAuthAppSpec;
