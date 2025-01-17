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
			"### Step 1: Create a New Application at Developer.wordpress.com:\n" +
			"If you already have an app, you can skip to Step 2.\n\n" +
			"- Ensure you are logged into your preferred Wordpress.com account.\n" +
			"- You can create and manage your apps at the [Application Manager](https://developer.wordpress.com/apps/) page.\n" +
			"- Click **Create New Application** on the top right corner.\n" +
			"- Enter a **Name**, **Description**, and **Website** for your application.\n" +
			"- Copy the url below and paste it into the **Redirect URL** field.\n",
	},
	{
		type: "copy",
		text: getOAuthLinks("wordpress").redirectURL,
	},
	{
		type: "markdown",
		text:
			"- Leave **Javascript Origins** blank for now, but you can update it later.\n" +
			"- For **Type**, select **Web**.\n" +
			"- Click **Create**.\n" +
			"- You will be redirected to the **Manage Application** page of the app you just created.\n",
	},
	{
		type: "markdown",
		text:
			"### Step 2: Register your App with Obot\n" +
			"- In the **Manage Application** page, you can find the **Client ID** and **Client Secret** in the **OAuth Information** section.\n" +
			"- Copy and paste them into the respective fields below.\n",
	},
	{ type: "input", input: "clientID", label: "Client ID" },
	{
		type: "input",
		input: "clientSecret",
		label: "Client Secret",
		inputType: "password",
	},
	{
		type: "markdown",
		text:
			"### (Optional) Create a New Site\n" +
			"If you don't have a site yet, you can create one by following these steps:\n" +
			"- Visit [WordPress Sites](https://wordpress.com/sites).\n" +
			"- Click **Add New Site** in the top-right corner, and follow the instructions to create a new site.\n",
	},
];

export const WordPressOAuthApp = {
	schema,
	alias: "wordpress",
	type: "wordpress",
	displayName: "WordPress",
	logo: assetUrl("/assets/wordpress-logo.png"),
	steps: steps,
	noGatewayIntegration: true,
} satisfies OAuthAppSpec;
