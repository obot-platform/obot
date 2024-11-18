import {
    Cell,
    ColumnDef,
    SortingState,
    flexRender,
    getCoreRowModel,
    getSortedRowModel,
    useReactTable,
} from "@tanstack/react-table";

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
}: DataTableProps<TData, TValue>) {
    const table = useReactTable({
        data,
        columns,
        state: { sorting: sort },
        getCoreRowModel: getCoreRowModel(),
        getSortedRowModel: getSortedRowModel(),
    });

    return (
        <div className="rounded-md max-h-full overflow-auto">
            <Table className="h-full">
                <TableHeader className="sticky top-0">
                    {table.getHeaderGroups().map((headerGroup) => (
                        <TableRow key={headerGroup.id} className="p-4">
                            {headerGroup.headers.map((header) => {
                                return (
                                    <TableHead key={header.id}>
                                        {header.isPlaceholder
                                            ? null
                                            : flexRender(
                                                  header.column.columnDef
                                                      .header,
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
                                className={cn(
                                    classNames?.row,
                                    rowClassName?.(row.original)
                                )}
                            >
                                {row.getVisibleCells().map(renderCell)}
                            </TableRow>
                        ))
                    ) : (
                        <TableRow className={cn(classNames?.row)}>
                            <TableCell
                                colSpan={columns.length}
                                className={cn(
                                    "h-24 text-center",
                                    classNames?.row
                                )}
                            >
                                No results.
                            </TableCell>
                        </TableRow>
                    )}
                </TableBody>
            </Table>
        </div>
    );

    function renderCell(cell: Cell<TData, TValue>) {
        return (
            <TableCell
                key={cell.id}
                className={cn("py-4", classNames?.cell, {
                    "cursor-pointer": !!onRowClick,
                })}
                onClick={() => {
                    if (!disableClickPropagation?.(cell)) {
                        onRowClick?.(cell.row.original);
                    }
                }}
            >
                {flexRender(cell.column.columnDef.cell, cell.getContext())}
            </TableCell>
        );
    }
}
