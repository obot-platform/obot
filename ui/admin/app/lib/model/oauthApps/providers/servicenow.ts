import { z } from "zod";

import {
    OAuthAppSpec,
    OAuthFormStep,
} from "~/lib/model/oauthApps/oauth-helpers";

const schema = z.object({
    clientID: z.string().min(1, "Client ID is required"),
    clientSecret: z.string().min(1, "Client Secret is required"),
    instanceURL: z.string().min(1, "Instance URL is required"),
});

const steps: OAuthFormStep<typeof schema.shape>[] = [
    // TODO(njhale): Add instructions for how to set up the OAuth App in ServiceNow and get
    // the required values below.
    { type: "input", input: "clientID", label: "Consumer Key" },
    {
        type: "input",
        input: "clientSecret",
        label: "Consumer Secret",
        inputType: "password",
    },
    { type: "input", input: "instanceURL", label: "Instance URL" },
];

export const ServiceNowOAuthApp = {
    schema,
    alias: "servicenow",
    type: "servicenow",
    displayName: "ServiceNow",
    steps: steps,
    noGatewayIntegration: true,
} satisfies OAuthAppSpec;
