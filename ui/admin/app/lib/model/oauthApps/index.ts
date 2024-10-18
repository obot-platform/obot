import { EntityMeta } from "~/lib/model/primitives";

import { GitHubOAuthApp } from "./github";
import { OAuthProvider, OAuthSingleAppSpec } from "./oauth-helpers";

export const OAuthAppInfo = {
    [OAuthProvider.GitHub]: GitHubOAuthApp,
} as const;

export type CombinedOAuthAppInfo = OAuthSingleAppSpec & {
    customApp?: OAuthApp;
};

export const combinedOAuthAppInfo = (apps: OAuthApp[]) => {
    return Object.entries(OAuthAppInfo).map(([type, defaultSpec]) => {
        const customApp = apps.find((app) => app.type === type);

        return { ...defaultSpec, customApp } as CombinedOAuthAppInfo;
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
    integration?: string;
};

export type OAuthAppBase = OAuthAppParams & {
    type: OAuthProvider;
    refName: string;
};

export type OAuthApp = EntityMeta &
    OAuthAppBase & {
        refNameAssigned?: boolean;
    };

export type OAuthAppInfo = {
    displayName: string;
    icon?: string;
    parameters: Record<keyof OAuthAppParams, string>;
};

export type OAuthAppSpec = Record<string, OAuthAppInfo>;
