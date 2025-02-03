import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";
import { PaginationParams, PaginationService } from "~/lib/service/pagination";

import { TableNamespace, WorkspaceTable } from "~/components/model/tables";

async function getTables(
	namespace: TableNamespace,
	entityId: string,
	pagination?: PaginationParams,
	search?: string
) {
	const { data } = await request<{ tables: Nullish<WorkspaceTable[]> }>({
		url: ApiRoutes.workspace.getTables(namespace, entityId).url,
	});

	const items = data.tables ?? [];

	const filtered = search
		? items.filter((table) => table.name.includes(search))
		: items;

	return PaginationService.paginate(filtered, pagination);
}
getTables.key = (
	namespace: TableNamespace,
	entityId: Nullish<string>,
	pagination?: PaginationParams,
	search?: string
) => {
	if (!entityId) return null;

	return {
		url: ApiRoutes.workspace.getTables(namespace, entityId).path,
		namespace,
		entityId,
		pagination,
		search,
	};
};

export const WorkspaceTableApiService = {
	getTables,
};
