import { EntityMeta } from "./primitives";

export const OAuthAppType = {
    Microsoft365: "microsoft365",
    Slack: "slack",
    Notion: "notion",
    HubSpot: "hubspot",
    GitHub: "github",
    Google: "google",
    Custom: "custom",
} as const;
export type OAuthAppType = (typeof OAuthAppType)[keyof typeof OAuthAppType];

export type OAuthAppParams = {
    refName?: string;
    name?: string;

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
    type: OAuthAppType;
};

export type OAuthApp = EntityMeta & OAuthAppBase;

export type OAuthAppSpec = Record<
    OAuthAppType,
    { displayName: string; parameters: Record<keyof OAuthAppParams, string> }
>;
