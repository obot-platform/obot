import { z } from "zod";

import {
	OAuthAppSpec,
	OAuthFormStep,
	getOAuthLinks,
} from "~/lib/model/oauthApps/oauth-helpers";
import { assetUrl } from "~/lib/utils";

const schema = z.object({
	appID: z.string().min(1, "App ID is required"),
	clientID: z.string().min(1, "Client ID is required"),
	clientSecret: z.string().min(1, "Client Secret is required"),
	optionalScope: z.string().optional(),
});

const steps: OAuthFormStep<typeof schema.shape>[] = [
	{
		type: "markdown",
		text:
			"### Step 1: Create a HubSpot Developer Account\n" +
			"- Go to step 2 if you already have a developer account.\n" +
			"- Complete the process to register a developer account.'\n" +
			"- https://app.hubspot.com/signup-hubspot/developers",
	},
	{
		type: "markdown",
		text:
			"### Step 2: Create app\n" +
			"- From inside the developer portal, select `Apps` from the left sidebar.\n" +
			"- Next, select `Create app` from the top right.\n" +
			"- Type in a name for your OAuth app. Optionally provide a logo and description.\n",
	},
	{
		type: "copy",
		text: getOAuthLinks("hubspot").redirectURL,
	},
	{
		type: "markdown",
		text:
			"### Step 3: Configure Auth Settings\n" +
			"- Select the `Auth` tab.\n" +
			"- Enter your Obot redirect URL into the `Redirect URLs` section.\n" +
			"- Add scopes:\n" +
			"  - crm.lists.read\n" +
			"  - crm.lists.write\n" +
			"  - crm.objects.companies.read\n" +
			"  - crm.objects.companies.write\n" +
			"  - crm.objects.contacts.read\n" +
			"  - crm.objects.contacts.write\n" +
			"  - crm.objects.deals.read\n" +
			"  - crm.objects.deals.write\n" +
			"  - crm.objects.owners.read\n" +
			"  - sales-email-read\n" +
			"  - tickets\n" +
			"- Mark each scope as optional.\n" +
			"- Click `Create app`",
	},
	{
		type: "markdown",
		text:
			"### Step 4: Register your HubSpot App credentials with Obot\n" +
			"- Select the `Auth` tab.\n" +
			"- Enter the `App ID, Client ID, Client Secret` from this page into the fields below.\n" +
			"- Also enter the scopes you enabled in the app creation process. Separate each scope with a space.",
	},
	{ type: "input", input: "appID", label: "App ID" },
	{ type: "input", input: "clientID", label: "Client ID" },
	{
		type: "input",
		input: "clientSecret",
		label: "Client Secret",
		inputType: "password",
	},
	{ type: "input", input: "optionalScope", label: "Optional Scope" },
];

export const HubSpotOAuthApp = {
	schema,
	alias: "hubspot",
	type: "hubspot",
	displayName: "HubSpot",
	logo: assetUrl("/assets/hubspot_logo.svg"),
	steps: steps,
	noGatewayIntegration: true,
} satisfies OAuthAppSpec;
