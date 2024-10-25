import { z } from "zod";

import { DomainUrl } from "~/lib/routers/baseRouter";
import { assetUrl } from "~/lib/utils";

import { OAuthAppSpec, OAuthFormStep, getOAuthLinks } from "../oauth-helpers";

const schema = z.object({
    clientID: z.string().min(1, "Client ID is required"),
    clientSecret: z.string().min(1, "Client Secret is required"),
});

const scopes = [
    "https://www.googleapis.com/auth/userinfo.email",
    "openid",
    "https://www.googleapis.com/auth/userinfo.profile",
    "https://www.googleapis.com/auth/spreadsheets",
    "https://www.googleapis.com/auth/documents.readonly",
    "https://mail.google.com/",
    "https://www.googleapis.com/auth/gmail.readonly",
    "https://www.googleapis.com/auth/gmail.compose",
];

const enabledApis = ["Google Sheets", "Google Docs", "Gmail"];

const steps: OAuthFormStep<typeof schema.shape>[] = [
    {
        type: "markdown",
        text:
            "### Step 1: Create a new Google Project\n" +
            "- Navigate to the [Credentials](https://console.cloud.google.com/apis/credentials) section of the APIs & Serivces page in your [Google API Dashboard](https://console.cloud.google.com).\n" +
            "- If you already have a Google Project Setup, skip to Step 2.",
    },
    {
        type: "sectionGroup",
        sections: [
            {
                title: "How do I create a new Google Project?",
                steps: [
                    {
                        type: "markdown",
                        text:
                            "1. Click on the `Create Project` Button.\n" +
                            "2. Enter a `Project Name`.\n" +
                            "3. Select a `Location`.\n" +
                            "4. Click on the `Create` Button.\n",
                    },
                ],
            },
        ],
    },
    {
        type: "markdown",
        text:
            "### Step 2: Enable APIs and Services.\n" +
            "- From the [Google API Dashboard](https://console.cloud.google.com/apis/dashboard), select your project from the dropdown in the top left, and click on the `+ ENABLE APIS AND SERVICES` button.\n" +
            "- Search for each of the required APIs and click on the `Enable` button .\n" +
            "- Repeat this process for each of the required APIs listed below.\n",
    },
    {
        type: "sectionGroup",
        sections: [
            {
                title: "Enable Apis: ",
                displayStepsInline: true,
                defaultOpen: true,
                steps: enabledApis.map((api) => ({
                    type: "copy",
                    text: api,
                })),
            },
        ],
    },
    {
        type: "markdown",
        text:
            "### Step 3: Configure your OAuth Consent Screen\n" +
            "Select one of the options below.",
    },
    {
        type: "sectionGroup",
        sections: [
            {
                title: "I have not already configured my OAuth Consent Screen\n",
                steps: [
                    {
                        type: "markdown",
                        text:
                            "- Click on the `OAuth consent screen` menu item on the left nav.\n" +
                            "- Select the `User type` (Recommended: `Internal`).\n" +
                            "  - If you need to use `External` you can read more about it [here](https://support.google.com/cloud/answer/10311615#user-type).\n" +
                            "- Click on the `Create` button. (You will be redirected to the `Edit app registration` screen.)\n",
                    },
                ],
            },
            {
                title: "I've already configured my OAuth Consent Screen",
                steps: [
                    {
                        type: "markdown",
                        text:
                            "- Click on the `OAuth consent screen` menu item on the left nav.\n" +
                            "- Click on the `Edit App` button.\n",
                    },
                ],
            },
        ],
    },
    {
        type: "markdown",
        text:
            "- (If you haven't already) Enter your `App Name`, `Support Email`, and optionally upload an image to the `App Logo` field.\n" +
            "- Under the `App Domain` section, add the url below to the `Application home page` field:\n",
    },
    {
        type: "copy",
        text: DomainUrl(),
    },
    {
        type: "markdown",
        text:
            "- Provide an email address to the `Developer contact information` field.\n" +
            "- Click on the `SAVE AND CONTINUE` button. (You will be redirected to the `Scopes` section.)\n" +
            "#### Scopes\n" +
            "- Click on the `Add or remove scopes` button, then add the following scopes listed below.\n" +
            "- Click on the `UPDATE` button when finished.\n",
    },
    {
        type: "sectionGroup",
        sections: [
            {
                title: "Scopes: ",
                displayStepsInline: true,
                defaultOpen: true,
                steps: scopes.map((scope) => ({
                    type: "copy",
                    text: scope,
                })),
            },
        ],
    },
    {
        type: "markdown",
        text: "- Click on the `SAVE AND CONTINUE` button to move on to Step 4.\n",
    },
    {
        type: "markdown",
        text:
            "### Step 4: Create OAuth Credentials\n" +
            "- Select the `Credentials` section from the left nav.\n" +
            "- Click on the `+ CREATE CREDENTIALS` button and select the `OAuth client ID` option.\n" +
            "- Select the `Web application` option from the `Application type` dropdown.\n" +
            "- Provide a `Name` for your OAuth client ID. (This is a label and will not be visible to users.)\n" +
            "- Click `+ ADD URI` button under the `Authorized redirect URIs` field and enter the url below:\n",
    },
    {
        type: "copy",
        text: getOAuthLinks("google").redirectURL,
    },
    {
        type: "markdown",
        text:
            "- Click on the `CREATE` button.\n" +
            "- Make sure to save the `Client ID` and `Client Secret` somewhere safe.\n",
    },
    {
        type: "markdown",
        text:
            "### Step 5: Register your OAuth App in Otto\n" +
            "With the credentials you just created, register your OAuth App in Otto by entering the `Client ID` and `Client Secret` into the fields below and clicking on the `Submit` button.",
    },
    { type: "input", input: "clientID", label: "Client ID" },
    {
        type: "input",
        input: "clientSecret",
        label: "Client Secret",
        inputType: "password",
    },
];

export const GoogleOAuthApp = {
    schema,
    refName: "google",
    type: "google",
    displayName: "Google",
    logo: assetUrl("/assets/google_logo.png"),
    steps: steps,
} satisfies OAuthAppSpec;
