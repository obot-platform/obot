import { z } from "zod";

import {
	OAuthAppSpec,
	OAuthFormStep,
	getOAuthLinks,
} from "~/lib/model/oauthApps/oauth-helpers";
import { assetUrl } from "~/lib/utils";

const schema = z.object({
	gitlabBaseURL: z.string().optional(),
	clientID: z.string().min(1, "Application ID is required"),
	clientSecret: z.string().min(1, "Secret is required"),
});

const steps: OAuthFormStep<typeof schema.shape>[] = [
	{
		type: "markdown",
		text:
			"### Step 1: Choose your GitLab instance\n" +
			"The default instance is the public GitLab.com. If you're using a self-hosted or enterprise GitLab instance, enter its base URL below:\n",
	},
	{
		type: "input",
		input: "gitlabBaseURL",
		label: "GitLab Base URL (optional, defaults to https://gitlab.com)",
	},
	{
		type: "markdown",
		text:
			"### Step 2: Create a new GitLab OAuth application\n" +
			"1. Go to your GitLab account, navigate to **Preferences > Applications**\n" +
			'2. Enter a **Name** for your application (e.g. "Obot Integration")\n' +
			"3. Fill in the **Redirect URI** with the link below\n",
	},
	{
		type: "copy",
		text: getOAuthLinks("gitlab").redirectURL,
	},
	{
		type: "markdown",
		text:
			"4. Select the required scopes:\n" +
			"   - `api` - for accessing GitLab API resources\n" +
			"   - `read_user` - for accessing user information\n" +
			"   - `email - access user email information\n",
	},
	{
		type: "markdown",
		text:
			"5. Click **Save application** to create the OAuth app\n" +
			"### Step 3: Register GitLab OAuth in Obot\n" +
			"1. Copy the **Application ID** from your GitLab OAuth app into the field below:\n",
	},
	{ type: "input", input: "clientID", label: "Application ID" },
	{
		type: "markdown",
		text: "2. Copy the **Secret** from your GitLab OAuth app into the field below:\n",
	},
	{
		type: "input",
		input: "clientSecret",
		label: "Secret",
		inputType: "password",
	},
];

export const GitLabOAuthApp = {
	schema,
	alias: "gitlab",
	type: "gitlab",
	displayName: "GitLab",
	logo: assetUrl("assets/gitlab_logo.svg"),
	steps,
} satisfies OAuthAppSpec;
