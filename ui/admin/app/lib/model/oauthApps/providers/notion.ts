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

const scopes = [
    "Read content",
    "Update content",
    "Insert content",
    "Read user information includeing email",
];

const steps: OAuthFormStep<z.infer<typeof schema>>[] = [
    {
        type: "markdown",
        text:
            "### Step 1: Create a new integration\n" +
            "If you already have an integration, you can skip to Step 2.\n\n" +
            "- Ensure you are logged into your preferred Notion account.\n" +
            "- From the [Notion Integrations Page](https://www.notion.so/profile/integrations), select **New Integration**.\n" +
            "- From the **Associated Workspace** dropdown menu, select the workspace you want to associate with this integration.\n",
    },
    {
        type: "sectionGroup",
        sections: [
            {
                title: "I don't have a workspace yet",
                steps: [
                    {
                        type: "markdown",
                        text:
                            "- If you have not created or joined a Notion workspace, you can follow the steps to [create a new workspace](https://www.notion.so/help/create-delete-and-switch-workspaces?fredir=1#create-a-new-workspace).\n\n" +
                            "- Once you have created your workspace, you can return to this page and select the workspace from the dropdown menu.\n",
                    },
                ],
            },
        ],
    },
    {
        type: "markdown",
        text:
            "- Add a **Name** for your integration.\n" +
            "- From the **Type** dropdown menu, select **Public**.\n" +
            "  - It's important to select **Public** for Otto to properly connect via OAuth.\n" +
            "- Enter the fields pertaining to your **company name**, **website**, **privacy policy**, and **terms of use**.\n" +
            "- Copy the url below and paste it into the **Redirect URI** field.\n",
    },
    {
        type: "copy",
        text: getOAuthLinks("notion").redirectURL,
    },
    {
        type: "markdown",
        text: "- Click **Save** at the bottom and then click **Configure Integration Settings** to continue.\n",
    },
    {
        type: "markdown",
        text:
            "### Step 2: Modifying your integration settings\n" +
            "- From the [Notion Integration Dashboard](https://www.notion.so/profile/integrations), select the integration you created and click **Edit Settings**.\n" +
            "(You will already be on this page if you've completed step 1)\n" +
            "- Ensure the **Capabilities** section has the following capabilities enabled:\n" +
            scopes.map((scope) => `  - **${scope}**\n`).join("") +
            "- Lastly, scroll to the top of the page and switch to the **Basic Information** tab.\n" +
            "- Scroll down to the **OAuth domains & URIs** section, and copy the url below into the **Redirect URIs** field.\n" +
            "- Click **Save** at the bottom to continue.\n",
    },
    {
        type: "markdown",
        text:
            "### Step 3: Register your integration with Otto\n" +
            "- Navigate to the **Configuration** tab from the top of the integration's settings page.\n" +
            "- Copy the **Client ID** and **Client Secret** and paste them into the respective fields below.\n",
    },
    { type: "input", input: "clientID", label: "Client ID" },
    {
        type: "input",
        input: "clientSecret",
        label: "Client Secret",
        inputType: "password",
    },
];

export const NotionOAuthApp = {
    schema,
    refName: "notion",
    type: "notion",
    displayName: "Notion",
    logo: assetUrl("/assets/notion_logo.png"),
    invertDark: true,
    steps,
} satisfies OAuthAppSpec;
