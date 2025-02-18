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
		type: "markdown",
		text:
			"### Step 2: Configure OAuth Settings\n" +
			"- Expand the 'Api (Enable OAuth Settings)' section, and check the box to Enable OAuth.\n" +
			"- Enter your callback url\n",
	},
	{
		type: "copy",
		text: getOAuthLinks("salesforce").redirectURL,
	},
	{
		type: "markdown",
		text:
			"- In the 'App Settings' section\n" +
			"  - Select the `Manage user data via APIs (api)` and `Perform requests at any time (refresh_token, offline_access)` OAuth Scopes from the list.\n" +
			"- In the 'Security' section\n" +
			"  - Uncheck 'Require Proof Key for Code Exchange.'\n" +
			"- Click the 'Create' button.\n",
	},
	{
		type: "markdown",
		text:
			"### Step 3: Configure App Policies\n" +
			"- Under the Policies tab, click 'Edit' and expand the 'OAuth Policies' section.\n" +
			"- Inside the 'App Authorization' box" +
			"  - (Optionally) Change 'IP Relaxation' to 'Relax IP Restrictions.'\n" +
			"- Click 'Save.'\n",
	},
	{
		type: "markdown",
		text:
			"### Step 4: Register your OAuth App credentials with Obot\n" +
			"- From the 'Settings' tab for your External Client App\n" +
			"  - Expand the 'OAuth Settings' box and then click 'Consumer Key and Secret' from inside the 'App Settings' block." +
			"- Enter the `Consumer Key and Secret` into the fields below, along with your Salesforce instance URL.\n",
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
