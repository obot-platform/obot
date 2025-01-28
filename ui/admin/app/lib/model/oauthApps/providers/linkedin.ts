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

const steps: OAuthFormStep<z.infer<typeof schema>>[] = [
	{
		type: "markdown",
		text:
			"### Step 1: Create a new app in LinkedIn Developer Portal\n" +
			"If you already have an app, you can skip to Step 2.\n\n" +
			"- Ensure you are logged in to your preferred LinkedIn account.\n" +
			"- From the [LinkedIn Developer Portal](https://developer.linkedin.com), click on **Create app**.\n" +
			"- Input required fields: *App Name* and *LinkedIn Page*, and upload an app logo, check the legal aggrement box and click on **Create App**.\n" +
			"- Now you should be redirected to the app's settings page.\n",
	},
	{
		type: "markdown",
		text:
			"### Step 2: Configure the app\n" +
			"- From the [LinkedIn Developer Portal](https://developer.linkedin.com), click **My Apps** and then select the app you'd like to proceed with.\n" +
			"(You will already be on this page if you've completed step 1)\n" +
			"- Select the **Products** tab and request access for **Share on LinkedIn** and **Sign In with LinkedIn using OpenID Connect**.\n" +
			"- Then select the **Auth** tab. In the **OAuth 2.0 settings** section, click on the pencil icon to set the following as redirect URL:\n",
	},
	{
		type: "copy",
		text: getOAuthLinks("linkedin").redirectURL,
	},
	{
		type: "markdown",
		text:
			"### Step 3: Register your App with Obot\n" +
			"- **Client ID** and **Client Secret** can be found in the **Application credentials** box, located at the top of the **Auth** tab page.\n" +
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

export const LinkedInOAuthApp = {
	schema,
	alias: "linkedin",
	type: "linkedin",
	displayName: "LinkedIn",
	logo: assetUrl("/assets/linkedin_icon.png"),
	steps: steps,
	noGatewayIntegration: true,
} satisfies OAuthAppSpec;
