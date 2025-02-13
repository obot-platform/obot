export const AuthDisabledUsername = "nobody";
export const BootstrapUsername = "bootstrap";
export const CommonAuthProviderIds = {
	GOOGLE: "google-auth-provider",
	GITHUB: "github-auth-provider",
	OKTA: "okta-auth-provider",
	ENTRA: "entra-auth-provider",
} as const;

export type CommonAuthProviderId =
	(typeof CommonAuthProviderIds)[keyof typeof CommonAuthProviderIds];
