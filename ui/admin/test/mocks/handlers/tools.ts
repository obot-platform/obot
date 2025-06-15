import { HttpResponse, http } from "test";
import {
	mockedBrowserToolBundle,
	mockedImageToolBundle,
	mockedKnowledgeToolReference,
	mockedTasksToolReference,
	mockedToolReferences,
	mockedWorkspaceFilesToolReference,
} from "test/mocks/models/toolReferences";

import { OAuthApp } from "~/lib/model/oauthApps";
import { EntityList } from "~/lib/model/primitives";
import { ToolReference } from "~/lib/model/toolReferences";
import { ApiRoutes } from "~/lib/routers/apiRoutes";

const toolReferences = {
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
	http.get(ApiRoutes.oauthApps.getOauthApps().path, () => {
		return HttpResponse.json<EntityList<OAuthApp>>({
			items: [],
		});
	}),
];
