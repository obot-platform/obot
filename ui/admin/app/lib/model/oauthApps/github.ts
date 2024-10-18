import { z } from "zod";

import {
    OAuthFormStep,
    OAuthSingleAppSpec,
    getOAuthLinks,
} from "./oauth-helpers";

const schema = z.object({
    authURL: z.string(),
    clientID: z.string(),
    clientSecret: z.string(),
    tokenURL: z.string().optional(),
});

const labels = {
    authURL: "Authorization URL",
    clientID: "Client ID",
    clientSecret: "Client Secret",
    tokenURL: "Token URL",
} satisfies Record<keyof typeof schema.shape, string>;

const steps: OAuthFormStep<typeof schema.shape>[] = [
    {
        type: "instruction",
        text:
            "#### Step 1: Create a new GitHub OAuth App\n" +
            "1. Navigate to [GitHub's Developer Settings](https://github.com/settings/developers) and select 'New OAuth App'.\n" +
            "2. The form will prompt you for an `Authorization callback Url` Make sure to use the link below: \n\n",
    },
    {
        type: "copy",
        text: getOAuthLinks("github").redirectURL,
    },
    {
        type: "instruction",
        text:
            "#### Step 2: Register OAuth App in Otto\n" +
            "Once you've created your OAuth App in GitHub, click 'Register application' and copy the client ID and client secret into this form",
    },
    { type: "input", input: "clientID", label: "Client ID" },
    { type: "input", input: "clientSecret", label: "Client Secret" },
];

export const GitHubOAuthApp = {
    schema,
    refName: "github",
    type: "github",
    displayName: "GitHub",
    labels,
    steps,
} satisfies OAuthSingleAppSpec;
