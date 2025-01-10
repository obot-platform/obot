import useSWR from "swr";

import { ModelProviderApiService } from "~/lib/service/api/modelProviderApiService";

export function useModelProviders() {
	const { data: modelProviders } = useSWR(
		ModelProviderApiService.getModelProviders.key(),
		() => ModelProviderApiService.getModelProviders()
	);
	const configured =
		modelProviders?.some((modelProvider) => modelProvider.configured) ?? false;

	return { configured, modelProviders: modelProviders ?? [] };
}
