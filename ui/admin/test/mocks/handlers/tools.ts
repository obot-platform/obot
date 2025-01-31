import { HttpResponse, http } from "test";
import {
	mockedBrowserToolBundle,
	mockedDatabaseToolReference,
	mockedImageToolBundle,
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
	"images-bundle": mockedImageToolBundle[0],
	"browser-bundle": mockedBrowserToolBundle[0],
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
