import { HttpResponse, http } from "test";

import { KnowledgeFile, KnowledgeSource } from "~/lib/model/knowledge";
import { EntityList } from "~/lib/model/primitives";
import { ApiRoutes } from "~/lib/routers/apiRoutes";

export const knowledgeHandlers = (entityId: string) => [
	http.get(
		ApiRoutes.knowledgeFiles.getKnowledgeFiles("agents", entityId).path,
		() => {
			return HttpResponse.json<EntityList<KnowledgeFile>>({
				items: [],
			});
		}
	),
	http.get(
		ApiRoutes.knowledgeSources.getKnowledgeSources("agents", entityId).path,
		() => {
			return HttpResponse.json<EntityList<KnowledgeSource> | null>({
				items: null,
			});
		}
	),
];
