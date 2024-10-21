import { z } from "zod";

import { OAuthAppSpec, OAuthFormStep, getOAuthLinks } from "./oauth-helpers";

const schema = z.object({
    clientID: z.string(),
    clientSecret: z.string(),
});

const steps: OAuthFormStep<typeof schema.shape>[] = [
    {
        type: "markdown",
        text: "### Step 1: Create a new GitHub OAuth App\n",
    },
    {
        type: "markdown",
        text:
            "#### If you haven't already, create a new GitHub OAuth App\n" +
            "1. Navigate to [GitHub's Developer Settings](https://github.com/settings/developers) and select `New OAuth App`.\n" +
            "2. The form will prompt you for an `Authorization callback Url` Make sure to use the link below: \n\n",
    },
    {
        type: "markdown",
        text:
            "#### If you already have a github OAuth app created\n" +
            "1. you can edit it by going to [Github's Developer Settings](https://github.com/settings/developers), and selecting `Edit` on your OAuth App\n" +
            "2. Near the bottom is the `Authorization callback URL` field. Make sure it matches the link below: \n\n",
    },
    {
        type: "copy",
        text: getOAuthLinks("github").redirectURL,
    },
    {
        type: "markdown",
        text:
            "### Step 2: Register OAuth App in Otto\n" +
            "Once you've created your OAuth App in GitHub, copy the client ID and client secret into this form",
    },
    { type: "input", input: "clientID", label: "Client ID" },
    { type: "input", input: "clientSecret", label: "Client Secret" },
];

export const GitHubOAuthApp = {
    schema,
    refName: "github",
    type: "github",
    displayName: "GitHub",
    steps,
} satisfies OAuthAppSpec;
