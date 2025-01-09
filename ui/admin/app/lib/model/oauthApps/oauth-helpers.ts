import { ZodObject, ZodType } from "zod";

import { ApiUrl } from "~/lib/routers/baseRouter";

export const OAuthProvider = {
    Atlassian: "atlassian",
    GitHub: "github",
    Google: "google",
    Microsoft365: "microsoft365",
    Slack: "slack",
    Salesforce: "salesforce",
    Notion: "notion",
    Zoom: "zoom",
    Custom: "custom",
} as const;
export type OAuthProvider = (typeof OAuthProvider)[keyof typeof OAuthProvider];

export type OAuthFormStep<T extends object = Record<string, string>> =
    | { type: "markdown"; text: string; copy?: string }
    | {
          type: "input";
          input: keyof T;
          label: string;
          inputType?: "password" | "text";
      }
    | { type: "copy"; text: string }
    | {
          type: "sectionGroup";
          sections: {
              title: string;
              steps: OAuthFormStep[];
              displayStepsInline?: boolean;
              defaultOpen?: boolean;
          }[];
      };

export type OAuthAppSpec = {
    schema: ZodObject<Record<string, ZodType>>;
    displayName: string;
    alias: string;
    type: OAuthProvider;
    logo?: string;
    darkLogo?: string;
    steps: OAuthFormStep[];
    disableConfiguration?: boolean;
    disabledReason?: string;
    invertDark?: boolean;
    noGatewayIntegration?: boolean;
};

export function getOAuthLinks(type: OAuthProvider) {
    return {
        authorizeURL: `${ApiUrl()}/app-oauth/authorize/${type}`,
        redirectURL: `${ApiUrl()}/app-oauth/callback/${type}`,
        refreshURL: `${ApiUrl()}/app-oauth/refresh/${type}`,
    };
}
