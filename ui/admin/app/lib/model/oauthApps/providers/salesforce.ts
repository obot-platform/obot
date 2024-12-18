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
    instanceURL: z.string().min(1, "Instance URL is required"),
});

const steps: OAuthFormStep<typeof schema.shape>[] = [
    {
        type: "markdown",
        text:
            "### Step 1: Create a new Salesforce External Client App\n" +
            "- Log in to your Salesforce portal.\n" +
            "- Go to Setup, and then search for 'External Client App Manager.'\n" +
            "- Select 'New External Client App' from the top right.\n" +
            "- Enter `Obot` as the External Client App Name.\n" +
            "- Fill in a Contact Email for your Salesforce Administrator.\n" +
            "- Set 'Distribution State' to 'Local' from the dropdown menu.\n" +
            "- (Optionally) Fill in the other fields.\n" +
            "- Click the `Create` button.\n",
    },
    {
        type: "copy",
        text: getOAuthLinks("salesforce").redirectURL,
    },
    {
        type: "markdown",
        text:
            "### Step 2: Configure OAuth Settings\n" +
            "- Expand the 'Api (Enable OAuth Settings)' section, and check the box to Enable OAuth.\n" +
            "- Enter your callback url\n" +
            "- Select the 'api' and 'refresh_token' OAuth Scopes from the list.\n" +
            "- Uncheck 'Require Proof Key for Code Exchange.'\n" +
            "- Check 'Enable Refresh Token Rotation.\n" +
            "- Click the 'Create' button.\n",
    },
    {
        type: "markdown",
        text:
            "### Step 3: Configure App Policies\n" +
            "- Under the Policies tab, click 'Edit'.\n" +
            "- Inside the 'App Authorization' box" +
            "  - Change 'Refresh Token Policy' to 'Immediately expire refresh token.'\n" +
            "  - (Optionally) Change 'IP Relaxation' to 'Relax IP Restrictions.'\n" +
            "- Click 'Save.'\n",
    },
    {
        type: "markdown",
        text:
            "### Step 4: Register your OAuth App credentials with Obot\n" +
            "- Navigate to the `Settings` tab in the sidebar.\n" +
            "- Enter the `Consumer Key and Secret` from the `Settings -> OAuth Settings` section into the fields below.\n" +
            "- Enter your Salesforce instance URL into the field below.\n",
    },
    { type: "input", input: "clientID", label: "Consumer Key" },
    {
        type: "input",
        input: "clientSecret",
        label: "Consumer Secret",
        inputType: "password",
    },
    { type: "input", input: "instanceURL", label: "Instance URL" },
];

export const SalesforceOAuthApp = {
    schema,
    alias: "salesforce",
    type: "salesforce",
    displayName: "Salesforce",
    logo: assetUrl("/assets/salesforce_logo.png"),
    steps: steps,
    noGatewayIntegration: true,
} satisfies OAuthAppSpec;
