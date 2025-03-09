import { z } from "zod";

import { EntityList } from "~/lib/model/primitives";
import { Project, ProjectShare } from "~/lib/model/project";
import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";
import {
	createFetcher,
	createMutator,
} from "~/lib/service/api/service-primitives";

const getAllFetcher = createFetcher(
	z.object({}),
	async (_, { signal }) => {
		const { url } = ApiRoutes.projects.getAll();
		const { data } = await request<EntityList<Project>>({ url, signal });
		return data.items ?? [];
	},
	() => ApiRoutes.projects.getAll().path
);

const getAllSharesFetcher = createFetcher(
	z.object({}),
	async (_, { signal }) => {
		const { url } = ApiRoutes.projectShares.getAll();
		const { data } = await request<EntityList<ProjectShare>>({ url, signal });
		return data.items ?? [];
	},
	() => ApiRoutes.projectShares.getAll().path
);

const deleteProjectMutator = createMutator(
	async ({ id, agentId }: { id: string; agentId: string }, { signal }) => {
		const { url } = ApiRoutes.projects.deleteProject(agentId, id);
		await request({ url, method: "DELETE", signal });
	}
);

export const ProjectApiService = {
	getAll: getAllFetcher,
	getAllShares: getAllSharesFetcher,
	delete: deleteProjectMutator,
};
