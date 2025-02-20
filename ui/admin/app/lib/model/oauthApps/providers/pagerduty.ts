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
			"### Step 1: Access PagerDuty Integrations\n" +
			"- Log in to your PagerDuty account\n" +
			"- Click on 'Integrations' in the top navigation\n" +
			"- Select 'App Registration' from the dropdown menu\n" +
			"- Click '+ New App'",
	},
	{
		type: "markdown",
		text:
			"### Step 2: Create OAuth App\n" +
			"- Fill in the app details:\n" +
			"  - Name: Choose a name for your integration (e.g., 'Obot')\n" +
			"  - Description: Brief description of how you'll use Obot\n" +
			"- Under 'Authentication Type', select 'OAuth 2.0'\n" +
			"- Click 'Next'",
	},
	{
		type: "copy",
		text: getOAuthLinks("pagerduty").redirectURL,
	},
	{
		type: "markdown",
		text:
			"### Step 3: Configure OAuth Settings\n" +
			"- In the OAuth Configuration section:\n" +
			"- Copy and paste your Obot redirect URL (shown above) into the 'Redirect URLs' field\n" +
			"- Select the following required scopes:\n" +
			"  - incidents.read\n" +
			"  - incidents.write\n" +
			"  - users.read\n" +
			"- Click 'Register App'",
	},
	{
		type: "markdown",
		text:
			"### Step 4: Get Your App Credentials\n" +
			"- After saving, you'll see your app's 'OAuth 2.0 Client Information'\n" +
			"- Copy the 'Client ID' and 'Client Secret'\n" +
			"- Enter these values in the fields below\n" +
			"- Download the 'Client Credentials' file and save it in a secure location",
	},
	{
		type: "input",
		input: "clientID",
		label: "Client ID",
	},
	{
		type: "input",
		input: "clientSecret",
		label: "Client Secret",
		inputType: "password",
	},
];

export const PagerDutyOAuthApp = {
	schema,
	alias: "pagerduty",
	type: "pagerduty",
	displayName: "PagerDuty",
	logo: assetUrl("/assets/pagerduty_logo.svg"),
	steps: steps,
} satisfies OAuthAppSpec;
