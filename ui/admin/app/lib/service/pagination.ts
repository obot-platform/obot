export type PaginationParams = {
	page: number;
	pageSize: number;
};

export type PaginationInfo = PaginationParams & {
	totalPages: number;
	total: number;
	nextPage?: number;
	previousPage?: number;
	firstPage: number;
	lastPage: number;
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

function getPaginationInfo(
	pagination: PaginationParams & { total: number }
): PaginationInfo {
	const totalPages = Math.ceil(pagination.total / pagination.pageSize);

	const nextPage =
		pagination.page + 1 >= totalPages ? undefined : pagination.page + 1;
	const previousPage =
		pagination.page - 1 >= 0 ? pagination.page - 1 : undefined;

	const firstPage = 0;
	const lastPage = totalPages - 1;

	return {
		...pagination,
		totalPages,
		nextPage,
		previousPage,
		firstPage,
		lastPage,
	};
}

type SearchableKey<T> = (item: T) => string;
type FuzzySearchParams<T> = {
	search?: string;
	key: SearchableKey<T>;
	caseSensitive?: boolean;
};
function handleSearch<T>(
	items: T[],
	{ search, key, caseSensitive }: FuzzySearchParams<T>
) {
	if (!search) return items;

	const withCase = (s: string) => (caseSensitive ? s : s.toLowerCase());
	return items.filter((item) => withCase(key(item)).includes(withCase(search)));
}

type SearchAndPaginateOptions<T> = {
	pagination?: PaginationParams;
	fuzzySearchParams?: FuzzySearchParams<T>;
};

function searchAndPaginate<T>(
	items: T[],
	{ pagination, fuzzySearchParams }: SearchAndPaginateOptions<T>
) {
	const filtered = fuzzySearchParams
		? handleSearch(items, fuzzySearchParams)
		: items;

	return paginate(filtered, pagination);
}

export const PaginationService = {
	paginate,
	getPaginationInfo,
	handleSearch,
	searchAndPaginate,
};
