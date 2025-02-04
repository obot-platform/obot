import { AuthProvider } from "~/lib/model/providers";

export const mockedAuthProvider: AuthProvider = {
	id: "google-auth-provider",
	created: "2025-02-04T16:04:02-05:00",
	revision: "1",
	type: "authprovider",
	name: "Google",
	toolReference: "github.com/obot-platform/tools/google-auth-provider",
	icon: "google_icon_small.png",
	iconDark: "google_icon_small.png",
	link: "https://google.com/",
	configured: true,
	requiredConfigurationParameters: [
		{
			name: "OBOT_GOOGLE_AUTH_PROVIDER_CLIENT_ID",
			friendlyName: "Client ID",
			description:
				"Unique identifier for the application when using Google's OAuth. Can typically be found in Google Cloud Console > Credentials",
		},
		{
			name: "OBOT_GOOGLE_AUTH_PROVIDER_CLIENT_SECRET",
			friendlyName: "Client Secret",
			description:
				"Password or key that app uses to authenticate with Google's OAuth. Can typically be found in Google Cloud Console > Credentials",
			sensitive: true,
		},
		{
			name: "OBOT_AUTH_PROVIDER_COOKIE_SECRET",
			friendlyName: "Cookie Secret",
			description:
				"Secret used to encrypt cookies. Must be a random string of length 16, 24, or 32.",
			sensitive: true,
		},
		{
			name: "OBOT_AUTH_PROVIDER_EMAIL_DOMAINS",
			friendlyName: "Allowed E-Mail Domains",
			description:
				"Comma separated list of email domains that are allowed to authenticate with this provider. * is a special value that allows all domains.",
		},
	],
};
