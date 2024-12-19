import {
    OAuthAppSpec,
    OAuthProvider,
} from "~/lib/model/oauthApps/oauth-helpers";
import { GitHubOAuthApp } from "~/lib/model/oauthApps/providers/github";
import { GoogleOAuthApp } from "~/lib/model/oauthApps/providers/google";
import { Microsoft365OAuthApp } from "~/lib/model/oauthApps/providers/microsoft365";
import { NotionOAuthApp } from "~/lib/model/oauthApps/providers/notion";
import { SlackOAuthApp } from "~/lib/model/oauthApps/providers/slack";
import { EntityMeta } from "~/lib/model/primitives";

export const OAuthAppSpecMap = {
    [OAuthProvider.GitHub]: GitHubOAuthApp,
    [OAuthProvider.Google]: GoogleOAuthApp,
    [OAuthProvider.Microsoft365]: Microsoft365OAuthApp,
    [OAuthProvider.Slack]: SlackOAuthApp,
    [OAuthProvider.Notion]: NotionOAuthApp,
    // Custom OAuth apps are intentionally omitted from the map.
    // They are handled separately
} as const;

export type OAuthAppDetail = OAuthAppSpec & {
    appOverride?: OAuthApp;
};

export const combinedOAuthAppInfo = (apps: OAuthApp[]) => {
    return Object.entries(OAuthAppSpecMap).map(([type, defaultSpec]) => {
        const appOverride = apps.find((app) => app.type === type);

        return { ...defaultSpec, appOverride } as OAuthAppDetail;
    });
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
    integration: string;
};

export type OAuthAppBase = OAuthAppParams & {
    name?: string;
    type: OAuthProvider;
    global: boolean;
};

export type CreateOAuthApp = Partial<OAuthAppBase> & {
    type: OAuthProvider;
    integration: string;
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
