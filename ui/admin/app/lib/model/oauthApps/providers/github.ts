import { z } from "zod";

import {
	OAuthAppSpec,
	OAuthFormStep,
	getOAuthLinks,
} from "~/lib/model/oauthApps/oauth-helpers";
import { BaseUrl } from "~/lib/routers/baseRouter";
import { assetUrl } from "~/lib/utils";

const schema = z.object({
	clientID: z.string().min(1, "Client ID is required"),
	clientSecret: z.string().min(1, "Client Secret is required"),
});

const steps: OAuthFormStep<typeof schema.shape>[] = [
	{
		type: "markdown",
		text:
			"### Step 1: Create a new GitHub OAuth App\n" +
			"1. In [GitHub's Developer Settings](https://github.com/settings/developers), select `New OAuth App`.\n" +
			"2. Specify an `Application name`\n" +
			"3. Fill in the `Homepage URL` with the link below\n",
	},
	{
		type: "copy",
		text: BaseUrl(),
	},

	{
		type: "markdown",
		text: "4. Fill in the `Authorization callback URL` with the link below\n",
	},
	{
		type: "copy",
		text: getOAuthLinks("github").redirectURL,
	},
	{
		type: "markdown",
		text:
			"5. Click `Register application` to create the OAuth app. It will now take you to the OAuth app's settings page.\n" +
			"### Step 2: Register GitHub OAuth in Obot\n" +
			"1. Locate the `Client ID` on the OAuth app's settings page and copy the `Client ID` into the input below\n",
	},
	{ type: "input", input: "clientID", label: "Client ID" },
	{
		type: "markdown",
		text:
			"2. Locate `Client Secrets` on the OAuth app's settings page, click `Generate new client secret`, and complete the authorization flow to generate a new secret.\n" +
			"3. Copy the newly generated `Client Secret` into the input below.",
	},
	{
		type: "input",
		input: "clientSecret",
		label: "Client Secret",
		inputType: "password",
	},
];

export const GitHubOAuthApp = {
	schema,
	alias: "github",
	type: "github",
	displayName: "GitHub",
	logo: assetUrl("assets/github_logo.png"),
	steps,
	invertDark: true,
} satisfies OAuthAppSpec;
