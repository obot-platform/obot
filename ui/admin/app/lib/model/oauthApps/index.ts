import {
	OAuthAppSpec,
	OAuthProvider,
} from "~/lib/model/oauthApps/oauth-helpers";
import { AtlassianOAuthApp } from "~/lib/model/oauthApps/providers/atlassian";
import { GitHubOAuthApp } from "~/lib/model/oauthApps/providers/github";
import { GoogleOAuthApp } from "~/lib/model/oauthApps/providers/google";
import { HubSpotOAuthApp } from "~/lib/model/oauthApps/providers/hubspot";
import { LinkedInOAuthApp } from "~/lib/model/oauthApps/providers/linkedin";
import { Microsoft365OAuthApp } from "~/lib/model/oauthApps/providers/microsoft365";
import { NotionOAuthApp } from "~/lib/model/oauthApps/providers/notion";
import { PagerDutyOAuthApp } from "~/lib/model/oauthApps/providers/pagerduty";
import { SalesforceOAuthApp } from "~/lib/model/oauthApps/providers/salesforce";
import { SlackOAuthApp } from "~/lib/model/oauthApps/providers/slack";
import { ZoomOAuthApp } from "~/lib/model/oauthApps/providers/zoom";
import { EntityMeta } from "~/lib/model/primitives";

export const OAuthAppSpecMap = {
	[OAuthProvider.Atlassian]: AtlassianOAuthApp,
	[OAuthProvider.GitHub]: GitHubOAuthApp,
	[OAuthProvider.Google]: GoogleOAuthApp,
	[OAuthProvider.HubSpot]: HubSpotOAuthApp,
	[OAuthProvider.Microsoft365]: Microsoft365OAuthApp,
	[OAuthProvider.Slack]: SlackOAuthApp,
	[OAuthProvider.Salesforce]: SalesforceOAuthApp,
	[OAuthProvider.Notion]: NotionOAuthApp,
	[OAuthProvider.Zoom]: ZoomOAuthApp,
	[OAuthProvider.LinkedIn]: LinkedInOAuthApp,
	[OAuthProvider.PagerDuty]: PagerDutyOAuthApp,
	// Custom OAuth apps are intentionally omitted from the map.
	// They are handled separately
} as const;

export type OAuthAppDetail = OAuthAppSpec & {
	appOverride?: OAuthApp;
};

export type OAuthAppParams = {
	clientID: string;
	clientSecret?: string;
	// These fields are only needed for custom OAuth apps.
	authURL?: string;
	tokenURL?: string;
	// This field is only needed for Microsoft 365 OAuth apps.
	tenantID?: string;
	// This field is only needed for HubSpot OAuth apps.
	appID?: string;
	// This field is optional for HubSpot OAuth apps.
	optionalScope?: string;
	// This field is required, it correlates to the integration name in the gptscript oauth cred tool
	alias: string;
	// This field is only needed for Salesforce OAuth apps
	instanceURL?: string;
};

export type OAuthAppBase = OAuthAppParams & {
	name?: string;
	type: OAuthProvider;
};

export type CreateOAuthApp = Partial<OAuthAppBase> & {
	type: OAuthProvider;
	alias: string;
};

export type OAuthApp = EntityMeta &
	OAuthAppBase & {
		aliasAssigned?: boolean;
		links: {
			authURL: string;
			tokenURL: string;
			redirectURL: string;
		};
	};
