import {
	Cell,
	ColumnDef,
	SortingState,
	flexRender,
	getCoreRowModel,
	getSortedRowModel,
	useReactTable,
} from "@tanstack/react-table";
import { useNavigate } from "react-router";
import { $path, RoutesWithParams } from "safe-routes";

import { cn } from "~/lib/utils";

import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "~/components/ui/table";

interface DataTableProps<TData, TValue> {
	columns: ColumnDef<TData, TValue>[];
	data: TData[];
	sort?: SortingState;
	rowClassName?: (row: TData) => string;
	classNames?: {
		row?: string;
		cell?: string;
	};
	onRowClick?: (row: TData) => void;
	onCtrlClick?: (row: TData) => void;
	disableClickPropagation?: (cell: Cell<TData, TValue>) => boolean;
}

export function DataTable<TData, TValue>({
	columns,
	data,
	sort,
	rowClassName,
	classNames,
	disableClickPropagation,
	onRowClick,
	onCtrlClick,
}: DataTableProps<TData, TValue>) {
	const table = useReactTable({
		data,
		columns,
		state: { sorting: sort },
		getCoreRowModel: getCoreRowModel(),
		getSortedRowModel: getSortedRowModel(),
	});

	return (
		<Table className="h-full">
			<TableHeader className="sticky top-0 z-10 bg-background">
				{table.getHeaderGroups().map((headerGroup) => (
					<TableRow key={headerGroup.id} className="p-4">
						{headerGroup.headers.map((header) => {
							return (
								<TableHead key={header.id}>
									{header.isPlaceholder
										? null
										: flexRender(
												header.column.columnDef.header,
												header.getContext()
											)}
								</TableHead>
							);
						})}
					</TableRow>
				))}
			</TableHeader>

			<TableBody>
				{table.getRowModel().rows?.length ? (
					table.getRowModel().rows.map((row) => (
						<TableRow
							key={row.id}
							data-state={row.getIsSelected() && "selected"}
							className={cn(classNames?.row, rowClassName?.(row.original))}
						>
							{row.getVisibleCells().map(renderCell)}
						</TableRow>
					))
				) : (
					<TableRow className={cn(classNames?.row)}>
						<TableCell
							colSpan={columns.length}
							className={cn("h-24 text-center", classNames?.row)}
						>
							No results.
						</TableCell>
					</TableRow>
				)}
			</TableBody>
		</Table>
	);

	function renderCell(cell: Cell<TData, TValue>) {
		return (
			<TableCell
				key={cell.id}
				className={cn("py-4", classNames?.cell, {
					"cursor-pointer": !!onRowClick,
				})}
				onClick={(e) => {
					if (disableClickPropagation?.(cell)) return;
					if (e.ctrlKey || e.metaKey) {
						onCtrlClick?.(cell.row.original);
					} else {
						onRowClick?.(cell.row.original);
					}
				}}
			>
				{flexRender(cell.column.columnDef.cell, cell.getContext())}
			</TableCell>
		);
	}
}

export const useRowNavigate = <TData extends Record<string, unknown>>(
	url: keyof RoutesWithParams,
	property: keyof TData &
		keyof {
			[K in keyof TData as TData[K] extends string | number
				? K
				: never]: TData[K];
		}
) => {
	const navigate = useNavigate();

	const handleAction = (row: TData, ctrl: boolean) => {
		const path = $path(url, { id: String(row[property]) });

		if (ctrl) {
			window.open(`/admin${path}`, "_blank");
		} else {
			navigate(path);
		}
	};

	return {
		onRowClick: (row: TData) => handleAction(row, false),
		onCtrlClick: (row: TData) => handleAction(row, true),
	};
};
