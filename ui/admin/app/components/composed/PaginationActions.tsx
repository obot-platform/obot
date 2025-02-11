import { ChevronLeft, ChevronRight } from "lucide-react";

import { Button } from "~/components/ui/button";

export type PaginationActionsProps = {
	page: number;
	nextPage?: number;
	previousPage?: number;
	totalPages: number;
	setPage: (page: number) => void;
};

export function PaginationActions({
	page,
	nextPage,
	previousPage,
	totalPages,
	setPage,
}: PaginationActionsProps) {
	const hasNextPage = nextPage != null;
	const hasPreviousPage = previousPage != null;

	const currentPage = totalPages ? page + 1 : 0;

	return (
		<div className="flex flex-nowrap items-center justify-center gap-2">
			<Button
				variant="ghost"
				size="icon-sm"
				disabled={!hasPreviousPage}
				onClick={() => hasPreviousPage && setPage(previousPage)}
			>
				<ChevronLeft />
			</Button>

			<p className="min-w-fit">
				{currentPage} / {totalPages}
			</p>

			<Button
				variant="ghost"
				size="icon-sm"
				disabled={!hasNextPage}
				onClick={() => hasNextPage && setPage(nextPage)}
			>
				<ChevronRight />
			</Button>
		</div>
	);
}
