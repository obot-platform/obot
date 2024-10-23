import { z } from "zod";

import { assetUrl } from "~/lib/utils";

import { OAuthAppSpec } from "../oauth-helpers";

const schema = z.object({
    clientID: z.string().min(1, "Client ID is required"),
    clientSecret: z.string().min(1, "Client Secret is required"),
});

export const Microsoft365OAuthApp = {
    schema,
    refName: "microsoft365",
    type: "microsoft365",
    displayName: "Microsoft 365",
    logo: assetUrl("/assets/office365_logo.svg"),
    steps: [],
    disableConfiguration: true,
} satisfies OAuthAppSpec;
