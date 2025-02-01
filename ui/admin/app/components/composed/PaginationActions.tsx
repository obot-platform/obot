import { ChevronLeft, ChevronRight } from "lucide-react";

import { Button } from "~/components/ui/button";

export type PaginationActionsProps = {
	page: number;
	nextPage?: number;
	previousPage?: number;
	totalPages: number;
	onPageChange: (page: number) => void;
};

export function PaginationActions({
	page,
	nextPage,
	previousPage,
	totalPages,
	onPageChange,
}: PaginationActionsProps) {
	const hasNextPage = nextPage != null;
	const hasPreviousPage = previousPage != null;

	return (
		<div className="flex flex-nowrap items-center justify-center gap-2">
			<Button
				variant="ghost"
				size="icon-sm"
				disabled={!hasPreviousPage}
				onClick={() => hasPreviousPage && onPageChange(previousPage)}
			>
				<ChevronLeft />
			</Button>

			<p className="min-w-fit">
				{page + 1} / {totalPages}
			</p>

			<Button
				variant="ghost"
				size="icon-sm"
				disabled={!hasNextPage}
				onClick={() => hasNextPage && onPageChange(nextPage)}
			>
				<ChevronRight />
			</Button>
		</div>
	);
}
