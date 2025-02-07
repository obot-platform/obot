import { z } from "zod";

import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";
import { createFetcher } from "~/lib/service/api/service-primitives";
import { QueryService } from "~/lib/service/queryService";

import {
	TableNamespace,
	WorkspaceTable,
	WorkspaceTableRows,
} from "~/components/model/tables";

const param = (x: string) => x as Todo;

const Keys = {
	getTables: (namespace: TableNamespace, entityId: string) => [
		namespace,
		entityId,
		"tables",
	],
	getTableRows: (
		namespace: TableNamespace,
		entityId: string,
		tableName: string
	) => [...Keys.getTables(namespace, entityId), tableName],
};

const getTables = createFetcher(
	QueryService.queryable.extend({
		namespace: z.nativeEnum(TableNamespace),
		entityId: z.string(),
		filters: z.object({ search: z.string() }).partial().nullish(),
	}),
	async ({ namespace, entityId, filters, query }) => {
		const { data } = await request<{ tables: Nullish<WorkspaceTable[]> }>({
			url: ApiRoutes.workspace.getTables(namespace, entityId).url,
		});

		const items = data.tables ?? [];
		const searched = QueryService.handleSearch(items, {
			key: (table) => table.name,
			search: filters?.search,
		});

		return QueryService.paginate(searched, query.pagination);
	},
	() => ApiRoutes.workspace.getTables(param(":namespace"), ":entityId").path
);

const getTableRows = createFetcher(
	QueryService.queryable.extend({
		namespace: z.nativeEnum(TableNamespace),
		entityId: z.string(),
		tableName: z.string(),
		filters: z.object({ search: z.string().optional() }).optional(),
	}),
	async (
		{ namespace, entityId, tableName, filters, query },
		{ signal } = {}
	) => {
		const { data } = await request<WorkspaceTableRows>({
			url: ApiRoutes.workspace.getTableRows(namespace, entityId, tableName).url,
			signal,
		});

		const searched = QueryService.handleSearch(data.rows ?? [], {
			key: (row) => Object.values(row).join("|"),
			search: filters?.search,
		});

		const { items: rows, ...rest } = QueryService.paginate(
			searched,
			query.pagination
		);

		data.rows = rows;

		return { ...data, ...rest };
	},
	() =>
		ApiRoutes.workspace.getTableRows(
			param(":namespace"),
			":entityId",
			":tableName"
		).path
);

export const WorkspaceTableApiService = {
	getTables,
	getTableRows,
};
