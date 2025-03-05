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

const scopes = [
	"user:read:user",
	"meeting:read:summary",
	"meeting:read:invitation",
	"meeting:read:list_templates",
	"meeting:read:meeting",
	"meeting:read:list_upcoming_meetings",
	"meeting:delete:meeting",
	"meeting:update:meeting",
	"meeting:write:meeting",
	"meeting:read:list_meetings",
	"meeting:read:past_meeting",
	"meeting:read:list_past_instances",
	"cloud_recording:read:list_recording_files",
	"cloud_recording:read:list_user_recordings",
];

const steps: OAuthFormStep<z.infer<typeof schema>>[] = [
	{
		type: "markdown",
		text:
			"### Step 1: Create a new app in Zoom App Marketplace\n" +
			"If you already have an app, you can skip to Step 2.\n\n" +
			"- Ensure you are logged into your preferred Zoom account.\n" +
			"- From the [Zoom App Marketplace](https://marketplace.zoom.us/), hover over the **Develop** box on the top right corner and then select **Build App**.\n" +
			"- When asked `What kind of app are you creating?` in the pop-up window, select **General App** and click **Create**.\n" +
			"- Now You should be redirected to the **App Basic Information** page.\n" +
			"- In the **Select how the app is managed** section, make sure it is set to **User-managed**.\n" +
			"- Scroll down to the **Oauth Information** section, copy the url below and paste it into both the **OAuth Redirect URL** field and the **OAuth Allow Lists** field.\n",
	},
	{
		type: "copy",
		text: getOAuthLinks("zoom").redirectURL,
	},
	{
		type: "markdown",
		text: "- Click **Continue** at the bottom and then click **Scopes** on the left sidebar to continue.\n",
	},
	{
		type: "markdown",
		text:
			"### Step 2: Configure the app's necessary scopes\n" +
			"- From the [Zoom App Management Page](https://marketplace.zoom.us/user/build), click on the app you created and click **Scopes** on the left sidebar.\n" +
			"(You will already be on this page if you've completed step 1)\n" +
			"- Click on **Add Scopes** to add necessary scopes for the zoom tool. Ensure the following scopes are added:\n",
	},
	{
		type: "sectionGroup",
		sections: [
			{
				title: "Scopes: ",
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
		text:
			"### Step 3: Register your App with Obot\n" +
			"- **Client ID** and **Client Secret** can be found in the **App Credentials** box, located at the top of the left sidebar. Alternatively, you can find them in the **App Credentials** section of the **Basic Information** page.\n" +
			"- Copy the **Client ID** and **Client Secret** and paste them into the respective fields below.\n",
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
			"### (Optional): Set up Third-Party Calendar and Contacts Integration\n" +
			"This step is optional, but it is **recommended** to set up the calendar and contacts integration for a more seamless experience.\n" +
			"For example, this allows retreiving Zoom meetings that the user has been invited to from a third-party calendar (Google, Exchange, Office 365, Apple iCloud). [Reference](https://support.zoom.com/hc/en/article?id=zm_kb&sysparm_article=KB0068615)\n" +
			"1. Sign in to the [**Zoom Web Portal**](https://zoom.us/signin)\n" +
			"2. Click [**Profile**](https://zoom.us/profile) on the left sidebar\n" +
			"3. Under **Others**, in the **Calendar and Contact Integration** section, click **Configure Calendar and Contacts Service**.\n" +
			"4. Select the service you want to integrate with\n" +
			"5. Select the [**permissions for the service**](https://support.zoom.com/hc/en/article?id=zm_kb&sysparm_article=KB0068615#h_315f3b9f-cad0-4f58-b61e-2038423120f0)\n" +
			"6. Click **Next**\n" +
			"7. Follow the on-screen instructions to grant Zoom access to the calendar/contacts service.\n" +
			"	- **Google:** You will be directed to Google's sign-in page. Sign in to your Google account. Click **Allow** to let Zoom access your contacts and Google Calendar.\n" +
			"	- **Office 365 (Outlook):**\n" +
			"		- **Authorize with OAuth 2.0:** Ensure this option is checked. \n" +
			"		- [**Choose your permissions.**](https://support.zoom.com/hc/en/article?id=zm_kb&sysparm_article=KB0068615#h_3a42993c-e620-42fe-8da6-a4ee0bdf2fcd) \n" +
			"		- Configure the type of Office 365 service.\n\n" +
			"		**Note:** If your Zoom calendar authentication has been disconnected or expired, this may result in issues with event synchronization between Zoom and your third-party calendar. To avoid disruptions, you can sign up for email notifications. These notifications will remind you to reauthorize your calendar and will be sent to the email address associated with your calendar integration. Click [here](https://go.zoom.us/profile/setting?amp_device_id=6ad31d8d-0d04-4abe-bbf0-7d30b889bb62&tab=zoomMailCalendar) to enable notifications and restore event synchronization.\n" +
			"	- **Exchange:** \n\n" +
			"	    **Note:** When [impersonation account is enabled](https://support.zoom.com/hc/en/article?id=zm_kb&sysparm_article=KB0069503#h_01G81WMCGAQEP899T3DKXGW82F), you do not have to enter a password for calendar and contact integration, and all your meetings created from the Zoom web portal or on Outlook are synced to the Zoom app or web portal.\n" +
			"		- **Exchange login username or UPN:** Enter the username or UPN associated with your Exchange account.\n" +
			"		- **Exchange login password:** Enter the password associated with Exchange account.\n" +
			"		- **Exchange Version:** Select the version of Exchange. If you are uncertain of the Exchange Version, please contact your internal IT team for more information.\n" +
			"		- **EWS URL:** Enter your organization's EWS URL. Contact your internal IT team if you do not know the EWS URL.\n" +
			"\nAfter allowing access, you will be redirected back to the Zoom web portal, which will indicate the permissions for the calendar and contacts integration.\n",
	},
];

export const ZoomOAuthApp = {
	schema,
	alias: "zoom",
	type: "zoom",
	displayName: "Zoom",
	logo: assetUrl("/assets/zoom_logo.svg"),
	steps: steps,
	noGatewayIntegration: true,
} satisfies OAuthAppSpec;
