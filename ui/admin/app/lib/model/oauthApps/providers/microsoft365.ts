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
	tenantID: z.string().optional(),
});

const scopes = [
"Calendars.Read",
	"Calendars.Read.Shared",
	"Calendars.ReadWrite",
	"Calendars.ReadWrite.Shared",
	"Files.Read.All",
	"Files.ReadWrite",
	"Group.Read.All",
	"Group.ReadWrite.All",
	"GroupMember.Read.All",
	"Mail.Read",
	"Mail.ReadWrite",
	"Mail.Send",
	"MailboxSettings.Read",
	"offline_access",
	"openid",
	"User.Read",
];

const steps: OAuthFormStep<typeof schema.shape>[] = [
	{
		type: "markdown",
		text: `
### Step 1: Create a new application registration in Entra ID\n
If you have already created an application registration, you can skip to Step 2.\n
- Navigate to your [Entra Id App Registrations Page](https://portal.azure.com/#view/Microsoft_AAD_IAM/ActiveDirectoryMenuBlade/~/RegisteredApps).\n
  - Or go to [Azure Portal](https://portal.azure.com) -> [Entra ID](https://portal.azure.com/#view/Microsoft_AAD_IAM/ActiveDirectoryMenuBlade/~/Overview) -> Manage -> [App registrations](https://portal.azure.com/#view/Microsoft_AAD_IAM/ActiveDirectoryMenuBlade/~/RegisteredApps)\n
- Click on **+ New registration**.\n
- Enter your application's **Name** and select the **Supported account type**.\n
- Locate the **Redirect URI** field, and select **Web** from the **Select a platform** dropdown.\n
- Then copy the url below and paste it into the **Redirect URI** field.\n
        `,
	},
	{
		type: "copy",
		text: getOAuthLinks("microsoft365").redirectURL,
	},
	{
		type: "markdown",
		text: `
- Click **Register** and continue to the next step.\n
        `,
	},
	{
		type: "markdown",
		text: `
### Step 2: Configure the Redirect URI\n\n

If you already set your web redirect URI in step 1, you can skip to Step 3.\n

- Navigate to your [Entra Id App Registrations Page](https://portal.azure.com/#view/Microsoft_AAD_IAM/ActiveDirectoryMenuBlade/~/RegisteredApps) and select your application.\n
  - You will already be on this page if you completed the previous step.\n
- Select **Authentication** from the left sidebar.\n
- Locate the **Platform configuration** section, and select **+ Add a platform**.\n
- Select **Web** and paste the url below into the **Redirect URI** field and click **Configure** at the bottom of the drawer.\n
        `,
	},
	{
		type: "copy",
		text: getOAuthLinks("microsoft365").redirectURL,
	},
	{
		type: "markdown",
		text: `
### Step 3: Configure the Required Permissions\n
- From the left sidebar, under the **Manage** tab, navigate to **API permissions**.\n
- Locate the **Configured permissions** section and click **+ Add a permission**.\n
- Under the **Microsoft APIs** tab, search for **Microsoft Graph** and select it.\n
- Select the **Delegated permissions** option.\n
- Search and select all of the following permissions (Or confirm they are already selected):\n
        `,
	},
	{
		type: "sectionGroup",
		sections: [
			{
				title: "Required Permissions:",
				displayStepsInline: true,
				defaultOpen: true,
				steps: scopes.map((scope) => ({
					type: "copy",
					text: scope,
				})),
			},
		],
	},
	{
		type: "markdown",
		text: `
**Note:** You will need Admin approval for the Group related permissions.\n

### Step 4: Generate a new client secret\n
- From the **Certificates & secrets** page, click **+ New client secret**.\n
- (Optional) enter a **Description** and select an **Expiration** date.\n
- Click **Add** and copy the new client secret ("**Secret Value**") to your clipboard.\n
  - Make sure to save it somewhere as it will not be accessible after closing this page.\n
- Paste your "**Secret Value**" into the **Client Secret** field below.\n
        `,
	},
	{
		type: "input",
		input: "clientSecret",
		label: "Client Secret",
		inputType: "password",
	},
	{
		type: "markdown",
		text: `
- Navigate to the **Overview** page in your application registration and copy the **Application (client) ID**.\n
- Paste the **Application (client) ID** into the **Client ID** field below.\n
        `,
	},
	{
		type: "input",
		input: "clientID",
		label: "Client ID",
	},
	{
		type: "markdown",
		text: `
#### For Single-Tenant Applications Only!\n\n
If you do not have a multi-tenant application, you can skip this step.\n
- From the **Overview** page in your application registration, copy the **Directory (tenant) ID**.\n
- Paste the **Directory (tenant) ID** into the **Tenant ID** field below.\n
        `,
	},
	{
		type: "input",
		input: "tenantID",
		label: "Tenant ID",
	},
	{
		type: "markdown",
		text: `
- Click **Submit** to finish registering your Application with Obot.\n
        `,
	},
];

export const Microsoft365OAuthApp = {
	schema,
	alias: "microsoft365",
	type: "microsoft365",
	displayName: "Microsoft 365",
	logo: assetUrl("/assets/office365_logo.svg"),
	steps,
} satisfies OAuthAppSpec;
