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
			"### Step 1: Create a new OAuth-In app\n" +
			"- Download the [SmartThings CLI](https://github.com/SmartThingsCommunity/smartthings-cli).\n" +
			"- In the terminal, run `smartthings apps:create`.'\n" +
			"- Fill out the required fields. Make sure you enter the `redirect_url` from below.\n",
	},
	{
		type: "copy",
		text: getOAuthLinks("smartthings").redirectURL,
	},
	{
		type: "markdown",
		text:
			"### Step 2: Register your OAuth App credentials with Obot\n" +
			"- Enter the `Client ID and Client Secret` into the fields below.\n",
	},
	{ type: "input", input: "clientID", label: "Client ID" },
	{
		type: "input",
		input: "clientSecret",
		label: "Client Secret",
		inputType: "password",
	},
];

export const SmartThingsOAuthApp = {
	schema,
	alias: "smartthings",
	type: "smartthings",
	displayName: "SmartThings",
	logo: assetUrl("/assets/smartthings_icon.png"),
	steps: steps,
	noGatewayIntegration: true,
} satisfies OAuthAppSpec;
