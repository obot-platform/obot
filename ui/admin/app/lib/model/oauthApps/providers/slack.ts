import { z } from "zod";

import { assetUrl } from "~/lib/utils";

import { OAuthAppSpec } from "../oauth-helpers";

const schema = z.object({
    clientID: z.string().min(1, "Client ID is required"),
    clientSecret: z.string().min(1, "Client Secret is required"),
});

export const SlackOAuthApp = {
    schema,
    refName: "slack",
    type: "slack",
    displayName: "Slack",
    logo: assetUrl("/assets/slack_logo_light.png"),
    darkLogo: assetUrl("/assets/slack_logo_dark.png"),
    steps: [],
    disableConfiguration: true,
} satisfies OAuthAppSpec;
