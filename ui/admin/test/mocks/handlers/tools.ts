import { HttpResponse, http } from "test";
import {
	mockedDatabaseToolReference,
	mockedKnowledgeToolReference,
	mockedTasksToolReference,
	mockedToolReferences,
	mockedWorkspaceFilesToolReference,
} from "test/mocks/models/toolReferences";

import { EntityList } from "~/lib/model/primitives";
import { ToolReference } from "~/lib/model/toolReferences";
import { ApiRoutes } from "~/lib/routers/apiRoutes";

const toolReferences = {
	database: mockedDatabaseToolReference,
	knowledge: mockedKnowledgeToolReference,
	tasks: mockedTasksToolReference,
	"workspace-files": mockedWorkspaceFilesToolReference,
};

export const toolsHandlers = [
	...Object.entries(toolReferences).map(([id, toolReference]) =>
		http.get(ApiRoutes.toolReferences.getById(id).path, () => {
			return HttpResponse.json<ToolReference>(toolReference);
		})
	),
	http.get(ApiRoutes.toolReferences.base({ type: "tool" }).path, () => {
		return HttpResponse.json<EntityList<ToolReference>>({
			items: mockedToolReferences,
		});
	}),
];
