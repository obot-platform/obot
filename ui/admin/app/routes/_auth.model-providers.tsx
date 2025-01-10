import { MetaFunction } from "react-router";
import { preload } from "swr";

import { ModelProvider } from "~/lib/model/modelProviders";
import { DefaultModelAliasApiService } from "~/lib/service/api/defaultModelAliasApiService";
import { ModelApiService } from "~/lib/service/api/modelApiService";
import { RouteHandle } from "~/lib/service/routeHandles";

import { WarningAlert } from "~/components/composed/WarningAlert";
import { ModelProviderList } from "~/components/model-providers/ModelProviderLists";
import { CommonModelProviderIds } from "~/components/model-providers/constants";
import { DefaultModelAliasFormDialog } from "~/components/model/DefaultModelAliasForm";
import { useModelProviders } from "~/hooks/model-providers/useModelProviders";

export async function clientLoader() {
	await Promise.all([
		preload(ModelApiService.getModels.key(), ModelApiService.getModels),
		preload(
			DefaultModelAliasApiService.getAliases.key(),
			DefaultModelAliasApiService.getAliases
		),
	]);
	return null;
}

const sortModelProviders = (modelProviders: ModelProvider[]) => {
	return [...modelProviders].sort((a, b) => {
		const preferredOrder = [
			CommonModelProviderIds.OPENAI,
			CommonModelProviderIds.AZURE_OPENAI,
			CommonModelProviderIds.ANTHROPIC,
			CommonModelProviderIds.ANTHROPIC_BEDROCK,
			CommonModelProviderIds.XAI,
			CommonModelProviderIds.OLLAMA,
			CommonModelProviderIds.VOYAGE,
			CommonModelProviderIds.GROQ,
			CommonModelProviderIds.VLLM,
			CommonModelProviderIds.DEEPSEEK,
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

export default function ModelProviders() {
	const { configured: modelProviderConfigured, modelProviders } =
		useModelProviders();
	const sortedModelProviders = sortModelProviders(modelProviders);
	return (
		<div>
			<div className="relative px-8 pb-8">
				<div className="sticky top-0 z-10 flex flex-col gap-4 bg-background py-8">
					<div className="flex items-center justify-between">
						<h2 className="mb-0 pb-0">Model Providers</h2>
						<DefaultModelAliasFormDialog disabled={!modelProviderConfigured} />
					</div>
					{modelProviderConfigured ? (
						<div className="h-16 w-full" />
					) : (
						<WarningAlert
							title="No Model Providers Configured!"
							description="To use Obot's features, you'll need to
                                set up a Model Provider. Select and configure
                                one below to get started!"
						/>
					)}
				</div>

				<div className="flex h-full flex-col gap-8 overflow-hidden">
					<ModelProviderList modelProviders={sortedModelProviders} />
				</div>
			</div>
		</div>
	);
}

export const handle: RouteHandle = {
	breadcrumb: () => [{ content: "Model Providers" }],
};

export const meta: MetaFunction = () => {
	return [{ title: `Obot • Model Providers` }];
};
