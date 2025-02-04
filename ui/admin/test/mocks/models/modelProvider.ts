import { ModelProvider } from "~/lib/model/providers";

export const mockedModelProvider: ModelProvider = {
	id: "openai-model-provider",
	created: "2025-02-04T16:04:02-05:00",
	revision: "1",
	type: "modelprovider",
	name: "OpenAI",
	toolReference: "github.com/obot-platform/tools/openai-model-provider",
	icon: "open-ai-logo-duotone.svg",
	link: "https://openai.com/",
	configured: true,
	requiredConfigurationParameters: [
		{
			name: "OBOT_OPENAI_MODEL_PROVIDER_API_KEY",
			friendlyName: "API Key",
			description:
				"OpenAI API Key. Can be created and fetched from https://platform.openai.com/settings/organization/api-keys or https://platform.openai.com/api-keys",
			sensitive: true,
		},
	],
	missingConfigurationParameters: ["OBOT_OPENAI_MODEL_PROVIDER_API_KEY"],
};
