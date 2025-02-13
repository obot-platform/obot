import { z } from "zod";

import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";
import { createFetcher } from "~/lib/service/api/service-primitives";
import { QueryService } from "~/lib/service/queryService";

import { TableNamespace, WorkspaceTable } from "~/components/model/tables";

const param = (x: string) => x as Todo;

const getTables = createFetcher(
	QueryService.queryable.extend({
		namespace: z.nativeEnum(TableNamespace),
		entityId: z.string(),
		filters: z.object({ search: z.string() }).partial().nullish(),
	}),
	async ({ namespace, entityId, filters, query }) => {
		const { data } = await request<WorkspaceTable[]>({
			url: ApiRoutes.workspace.getTables(namespace, entityId).url,
		});

		const items = data ?? [];
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
		const { data } = await request<Record<string, unknown>[]>({
			url: ApiRoutes.workspace.getTableRows(namespace, entityId, tableName).url,
			signal,
		});

		// Get unique column names from all rows
		const columns = Array.from(
			new Set((data ?? []).flatMap((row) => Object.keys(row)))
		);

		const searched = QueryService.handleSearch(data ?? [], {
			key: (row) => Object.values(row).join("|"),
			search: filters?.search,
		});

		const { items: rows, ...rest } = QueryService.paginate(
			searched,
			query.pagination
		);

		return { columns, rows, ...rest };
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
