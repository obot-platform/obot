import { useCallback, useState } from "react";

import { PaginationParams, QueryService } from "~/lib/service/queryService";

import { useDebounce } from "~/hooks/useDebounce";

type PaginationStoreProps = {
	initialPage?: number;
	initialSearch?: string;
	pageSize: number;
};

export function usePagination({
	initialPage = 0,
	initialSearch,
	pageSize,
}: PaginationStoreProps) {
	const [page, setPage] = useState(initialPage);
	const [search, setSearch] = useState(initialSearch);
	const [total, setTotal] = useState(pageSize);

	const updateSearch = useCallback((search: string) => {
		setSearch(search);
		setPage(0);
	}, []);

	const debouncedSearch = useDebounce(updateSearch, 500);

	const pagination = QueryService.getPaginationInfo({
		page,
		pageSize,
		total,
	});

	const updateTotal = useCallback(
		(newTotal?: number) => {
			if (newTotal != null && newTotal !== total) setTotal(newTotal);
		},
		[total]
	);

	const params = {
		pagination: { page, pageSize } as PaginationParams,
		search,
	};

	return {
		...pagination,
		params,
		search,
		setPage,
		setSearch: updateSearch,
		debouncedSearch,
		total,
		updateTotal,
	};
}
