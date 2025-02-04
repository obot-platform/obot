import { OpenInNewWindowIcon } from "@radix-ui/react-icons";
import { SearchIcon } from "lucide-react";
import { useState } from "react";

import { isArrayEqual } from "~/lib/utils/isArrayEqual";

import { useThreadTableRows } from "~/components/chat/shared/thread-helpers";
import { PaginationActions } from "~/components/composed/PaginationActions";
import { Button } from "~/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogHeader,
	DialogTitle,
	DialogTrigger,
} from "~/components/ui/dialog";
import { Input } from "~/components/ui/input";
import { Skeleton } from "~/components/ui/skeleton";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "~/components/ui/table";
import { usePagination } from "~/hooks/pagination/usePagination";

type ThreadTableDialogProps = {
	threadId: string;
	tableName: string;
};

const pageSize = 10;

export function ThreadTableDialog({
	threadId,
	tableName,
}: ThreadTableDialogProps) {
	const [open, setOpen] = useState(false);
	const [columns, setColumns] = useState<string[]>();

	const tableStore = usePagination({ pageSize });
	const getTableRows = useThreadTableRows({
		threadId,
		tableName,
		...tableStore.params,
		disabled: !open,
	});
	const { columns: _columns, rows, total } = getTableRows.data ?? {};

	if (_columns && !isArrayEqual(_columns, columns ?? [])) setColumns(_columns);

	tableStore.updateTotal(total);

	return (
		<Dialog open={open} onOpenChange={setOpen}>
			<DialogTrigger asChild>
				<Button size="icon" variant="ghost">
					<OpenInNewWindowIcon />
				</Button>
			</DialogTrigger>

			<DialogContent aria-describedby={undefined} className="max-w-screen-lg">
				<DialogHeader>
					<DialogTitle>{tableName}</DialogTitle>
				</DialogHeader>

				<Input
					placeholder="Search"
					startContent={<SearchIcon />}
					onChange={(e) => tableStore.debouncedSearch(e.target.value)}
				/>

				<Table>
					<TableHeader>
						<TableRow>{renderHeadCells()}</TableRow>
					</TableHeader>

					<TableBody>{renderRows()}</TableBody>
				</Table>

				<PaginationActions {...tableStore} />
			</DialogContent>
		</Dialog>
	);

	function renderHeadCells() {
		if (!columns) return renderSkeletonHead();
		return columns?.map((col) => <TableHead key={col}>{col}</TableHead>);
	}

	function renderRows() {
		if (!rows) return renderSkeletonRows(columns?.length);

		const dataRows = rows.map((row, index) => (
			<TableRow key={rowKey(row, index)}>
				{columns?.map((col) => (
					<TableCell key={col}>
						<p>{row[col]}</p>
					</TableCell>
				))}
			</TableRow>
		));

		const invisibleRows = Array.from(
			{ length: pageSize - dataRows.length },
			() => renderInvisibleRow()
		);

		return [...dataRows, ...invisibleRows];
	}

	function renderInvisibleRow() {
		return (
			<TableRow className="invisible border-none">
				<TableCell colSpan={columns?.length}>
					<p>.</p>
				</TableCell>
			</TableRow>
		);
	}

	function rowKey(row: Record<string, string>, index: number) {
		return `${index} ${Object.values(row).join("-")}`;
	}

	function renderSkeletonHead(cols = 3) {
		return Array.from({ length: cols }, (_, i) => (
			<TableHead key={i}>
				<Skeleton className="h-4 w-full">
					<p>.</p>
				</Skeleton>
			</TableHead>
		));
	}

	function renderSkeletonRows(cols = 3) {
		return Array.from({ length: pageSize }, (_, i) => (
			<TableRow key={i}>
				{Array.from({ length: cols }, (_, i) => (
					<TableCell key={i}>
						<Skeleton className="rounded-full">
							<p>.</p>
						</Skeleton>
					</TableCell>
				))}
			</TableRow>
		));
	}
}
