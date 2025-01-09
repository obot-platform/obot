import { Assistant } from "~/lib/model/assistants";
import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";

async function getAssistants() {
	const { data } = await request<{ items: Assistant[] }>({
		url: ApiRoutes.assistants.getAssistants().url,
	});

	return data.items ?? [];
}
getAssistants.key = () => ({ url: ApiRoutes.assistants.getAssistants().path });

export const AssistantApiService = {
	getAssistants,
};
