import { MetaFunction } from "react-router";

import { AuthProvider } from "~/lib/model/providers";
import { VersionApiService } from "~/lib/service/api/versionApiService";
import { RouteHandle } from "~/lib/service/routeHandles";

import { AuthProviderList } from "~/components/auth-and-model-providers/AuthProviderLists";
import { CommonAuthProviderIds } from "~/components/auth-and-model-providers/constants";
import { WarningAlert } from "~/components/composed/WarningAlert";
import { useAuthProviders } from "~/hooks/auth-providers/useAuthProviders";

export async function clientLoader() {
	await VersionApiService.requireAuthEnabled();
}

const sortAuthProviders = (authProviders: AuthProvider[]) => {
	return [...authProviders].sort((a, b) => {
		const preferredOrder = [
			CommonAuthProviderIds.GOOGLE,
			CommonAuthProviderIds.GITHUB,
			CommonAuthProviderIds.OKTA,
		];
		const aIndex = preferredOrder.indexOf(a.id);
		const bIndex = preferredOrder.indexOf(b.id);

		// If both providers are in preferredOrder, sort by their order
		if (aIndex !== -1 && bIndex !== -1) {
			return aIndex - bIndex;
		}

		// If only a is in preferredOrder, it comes first
		if (aIndex !== -1) return -1;
		// If only b is in preferredOrder, it comes first
		if (bIndex !== -1) return 1;

		// For all other providers, sort alphabetically by name
		return a.name.localeCompare(b.name);
	});
};

export default function AuthProviders() {
	const { configured: authProviderConfigured, authProviders } =
		useAuthProviders();
	const sortedAuthProviders = sortAuthProviders(authProviders);
	return (
		<div>
			<div className="relative px-8 pb-8">
				<div className="sticky top-0 z-10 flex flex-col gap-4 bg-background py-8">
					<div className="flex items-center justify-between">
						<h2 className="mb-0 pb-0">Auth Providers</h2>
					</div>
					{authProviderConfigured ? (
						<div className="h-16 w-full" />
					) : (
						<WarningAlert
							title="No Auth Providers Configured!"
							description="To finish setting up Obot, you'll need to
                                configure an Auth Provider. Select one below to get started!"
						/>
					)}
				</div>

				<div className="flex h-full flex-col gap-8 overflow-hidden">
					<AuthProviderList authProviders={sortedAuthProviders} />
				</div>
			</div>
		</div>
	);
}

export const handle: RouteHandle = {
	breadcrumb: () => [{ content: "Auth Providers" }],
};

export const meta: MetaFunction = () => {
	return [{ title: `Obot • Auth Providers` }];
};
