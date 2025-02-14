import {
	Cell,
	ColumnDef,
	SortingState,
	flexRender,
	getCoreRowModel,
	getSortedRowModel,
	useReactTable,
} from "@tanstack/react-table";
import { ListFilterIcon } from "lucide-react";
import { useState } from "react";
import { DateRange } from "react-day-picker";
import { useNavigate } from "react-router";

import { cn } from "~/lib/utils";

import { ComboBox } from "~/components/composed/ComboBox";
import { Button } from "~/components/ui/button";
import { Calendar } from "~/components/ui/calendar";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "~/components/ui/popover";
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
		enableColumnResizing: true,
		columnResizeMode: "onChange",
		columnResizeDirection: "ltr",
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
								<TableHead
									key={header.id}
									style={{ width: header.getSize() }}
									className="space-between group relative px-0"
								>
									<div className="flex h-full w-full items-center justify-between">
										{header.isPlaceholder ? null : (
											<div className="w-full px-2">
												{flexRender(
													header.column.columnDef.header,
													header.getContext()
												)}
											</div>
										)}
										{header.column.getCanResize() && (
											<button
												onMouseDown={header.getResizeHandler()}
												onTouchStart={header.getResizeHandler()}
												className={cn(
													"mx-2 h-full w-1 cursor-col-resize self-end group-hover:bg-muted-foreground/30",
													{
														isResizing: header.column.getIsResizing(),
													}
												)}
											></button>
										)}
									</div>
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
				style={{ width: cell.column.getSize() }}
			>
				{flexRender(cell.column.columnDef.cell, cell.getContext())}
			</TableCell>
		);
	}
}

export const useRowNavigate = <TData extends object | string>(
	getPath: (row: TData) => string
) => {
	const navigate = useNavigate();

	const handleAction = (row: TData, ctrl: boolean) => {
		const path = getPath(row);
		if (ctrl) {
			window.open(`/admin${path}`, "_blank");
		} else {
			navigate(path);
		}
	};

	return {
		internal: (row: TData) => handleAction(row, false),
		external: (row: TData) => handleAction(row, true),
	};
};

export const DataTableFilter = ({
	field,
	values,
	onSelect,
}: {
	field: string;
	onSelect: (value: string) => void;
	values: { id: string; name: string }[];
}) => {
	return (
		<ComboBox
			buttonProps={{
				className: "px-0 w-full",
				variant: "text",
				endContent: <ListFilterIcon />,
			}}
			placeholder={field}
			onChange={(option) => onSelect(option?.id ?? "")}
			options={values}
			classNames={{
				command: "min-w-64",
			}}
		/>
	);
};

export const DataTableTimeFilter = ({
	dateRange,
	field,
	onSelect,
}: {
	dateRange: DateRange;
	field: string;
	onSelect: (range: DateRange) => void;
}) => {
	const [range, setRange] = useState<DateRange | undefined>(dateRange);
	return (
		<Popover>
			<PopoverTrigger asChild>
				<Button
					variant="text"
					endContent={<ListFilterIcon />}
					className="w-full p-0"
					classNames={{
						content: "w-full justify-between",
					}}
				>
					{field}
				</Button>
			</PopoverTrigger>
			<PopoverContent>
				<Calendar
					mode="range"
					selected={range}
					onSelect={(range) => {
						setRange(range);
						if (range?.from && range?.to) {
							onSelect(range);
						}
					}}
					initialFocus
				/>
			</PopoverContent>
		</Popover>
	);
};
