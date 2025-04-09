import {
	Cell,
	ColumnDef,
	ExpandedState,
	GroupingState,
	SortingState,
	flexRender,
	getCoreRowModel,
	getExpandedRowModel,
	getSortedRowModel,
	useReactTable,
} from "@tanstack/react-table";
import { ListFilterIcon } from "lucide-react";
import { useState } from "react";
import { DateRange } from "react-day-picker";
import { FaCaretDown, FaCaretUp } from "react-icons/fa";
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
	grouping?: GroupingState;
	expanded?: ExpandedState;
	rowClassName?: (row: TData) => string;
	groupBy?: (row: TData, index: number) => TData[];
	classNames?: { row?: string; cell?: string };
	onRowClick?: (row: TData) => void;
	onCtrlClick?: (row: TData) => void;
	disableClickPropagation?: (cell: Cell<TData, TValue>) => boolean;
}

export function DataTable<TData, TValue>({
	columns,
	data,
	sort,
	expanded = true,
	rowClassName,
	classNames,
	disableClickPropagation,
	onRowClick,
	onCtrlClick,
	groupBy,
}: DataTableProps<TData, TValue>) {
	const [sorting, setSorting] = useState<SortingState>(sort ?? []);
	const table = useReactTable({
		enableColumnResizing: true,
		columnResizeMode: "onChange",
		columnResizeDirection: "ltr",
		data,
		columns,
		state: { sorting: sorting, expanded },
		onSortingChange: setSorting,
		getSubRows: groupBy,
		getCoreRowModel: getCoreRowModel(),
		getSortedRowModel: getSortedRowModel(),
		getExpandedRowModel: getExpandedRowModel(),
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
											<div className="flex w-full items-center justify-between px-2">
												{flexRender(
													header.column.columnDef.header,
													header.getContext()
												)}
												{header.column.id === "actions" ? null : (
													<button
														className="ml-2 flex-col items-center justify-center"
														onClick={() =>
															setSorting([
																{
																	id: header.column.id,
																	desc: !sorting[0]?.desc,
																},
															])
														}
													>
														<FaCaretUp
															className={
																header.column.getIsSorted() === "asc"
																	? "opacity-100"
																	: "opacity-20"
															}
														/>
														<FaCaretDown
															className={
																header.column.getIsSorted() === "desc"
																	? "opacity-100"
																	: "opacity-20"
															}
														/>
													</button>
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
				{table.getSortedRowModel().rows?.length ? (
					table.getSortedRowModel().rows.map((row) => (
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
	onSelect: (range?: DateRange) => void;
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
					classNames={{
						caption: "flex items-center justify-between gap-4 pl-2",
						nav: "flex gap-4",
					}}
					mode="range"
					selected={range}
					onSelect={(range) => {
						setRange(range);
					}}
					initialFocus
				/>
				<div className="flex justify-between gap-2">
					<Button
						variant="secondary"
						onClick={() => {
							setRange(undefined);
							onSelect(undefined);
						}}
					>
						Clear
					</Button>
					<Button onClick={() => onSelect(range)}>Apply</Button>
				</div>
			</PopoverContent>
		</Popover>
	);
};
