import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";
import { PaginationParams, PaginationService } from "~/lib/service/pagination";

import {
	TableNamespace,
	WorkspaceTable,
	WorkspaceTableRows,
} from "~/components/model/tables";

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

	return PaginationService.searchAndPaginate(items, {
		pagination,
		fuzzySearchParams: { search, key: (table) => table.name },
	});
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

async function getTableRows(
	namespace: TableNamespace,
	entityId: string,
	tableId: string,
	pagination?: PaginationParams,
	search?: string
) {
	const { data } = await request<WorkspaceTableRows>({
		url: ApiRoutes.workspace.getTableRows(namespace, entityId, tableId).url,
	});

	const items = data.rows ?? [];

	const { items: rows, ...rest } = PaginationService.searchAndPaginate(items, {
		pagination,
		fuzzySearchParams: { search, key: (row) => Object.values(row).join("|") },
	});

	data.rows = rows;

	return { ...data, ...rest };
}
getTableRows.key = (
	namespace: TableNamespace,
	entityId: Nullish<string>,
	tableId: Nullish<string>,
	pagination?: PaginationParams,
	search?: string
) => {
	if (!entityId || !tableId) return null;
	return {
		url: ApiRoutes.workspace.getTableRows(namespace, entityId, tableId).path,
		namespace,
		entityId,
		tableId,
		pagination,
		search,
	};
};

export const WorkspaceTableApiService = {
	getTables,
	getTableRows,
};
