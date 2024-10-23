import { z } from "zod";

import { assetUrl } from "~/lib/utils";

import { OAuthAppSpec } from "../oauth-helpers";

const schema = z.object({
    clientID: z.string().min(1, "Client ID is required"),
    clientSecret: z.string().min(1, "Client Secret is required"),
});

export const NotionOAuthApp = {
    schema,
    refName: "notion",
    type: "notion",
    displayName: "Notion",
    logo: assetUrl("/assets/notion_logo.png"),
    invertDark: true,
    steps: [],
    disableConfiguration: true,
} satisfies OAuthAppSpec;
