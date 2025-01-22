import useSWR from "swr";

import { AuthProviderApiService } from "~/lib/service/api/authProviderApiService";

export function useAuthProviders() {
	const { data: authProviders } = useSWR(
		AuthProviderApiService.getAuthProviders.key(),
		() => AuthProviderApiService.getAuthProviders()
	);
	const configured =
		authProviders?.some((authProvider) => authProvider.configured) ?? false;

	return { configured, authProviders: authProviders ?? [] };
}
