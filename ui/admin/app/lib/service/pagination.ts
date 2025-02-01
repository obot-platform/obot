export type PaginationParams = {
	page: number;
	pageSize: number;
};

export type PaginationInfo = PaginationParams & {
	totalPages: number;
	total: number;
	nextPage?: number;
	previousPage?: number;
};

export type Paginated<T> = {
	items: T[];
	total: number;
};

function paginate<T>(items: T[], pagination?: PaginationParams): Paginated<T> {
	if (!pagination) {
		return { items, total: items.length };
	}

	const start = pagination.page * pagination.pageSize;
	const end = start + pagination.pageSize;

	return { items: items.slice(start, end), total: items.length };
}

export function getPaginationInfo(
	pagination: PaginationParams & { total: number }
): PaginationInfo {
	const totalPages = Math.ceil(pagination.total / pagination.pageSize);

	const nextPage =
		pagination.page + 1 >= totalPages ? undefined : pagination.page + 1;
	const previousPage =
		pagination.page - 1 >= 0 ? pagination.page - 1 : undefined;

	return { ...pagination, totalPages, nextPage, previousPage };
}

export const PaginationService = {
	paginate,
	getPaginationInfo,
};
