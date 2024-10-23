import { z } from "zod";

import { assetUrl } from "~/lib/utils";

import { OAuthAppSpec } from "../oauth-helpers";

const schema = z.object({
    clientID: z.string().min(1, "Client ID is required"),
    clientSecret: z.string().min(1, "Client Secret is required"),
});

export const GoogleOAuthApp = {
    schema,
    refName: "google",
    type: "google",
    displayName: "Google",
    logo: assetUrl("/assets/google_logo.png"),
    steps: [],
    disableConfiguration: true,
} satisfies OAuthAppSpec;
