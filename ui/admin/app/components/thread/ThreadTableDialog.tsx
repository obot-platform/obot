import { OpenInNewWindowIcon } from "@radix-ui/react-icons";
import {
	AlertCircleIcon,
	ArrowUpIcon,
	CheckIcon,
	ChevronDown,
	SearchIcon,
} from "lucide-react";
import { useEffect, useState } from "react";
import { useForm } from "react-hook-form";

import { InvokeService } from "~/lib/service/api/invokeService";
import { cn } from "~/lib/utils";
import { isArrayEqual } from "~/lib/utils/isArrayEqual";

import { useThreadTableRows } from "~/components/chat/shared/thread-helpers";
import { PaginationActions } from "~/components/composed/PaginationActions";
import { ControlledAutosizeTextarea } from "~/components/form/controlledInputs";
import { LoadingSpinner } from "~/components/ui/LoadingSpinner";
import { AnimatePresence } from "~/components/ui/animate";
import { ExpandAndCollapse } from "~/components/ui/animate/expand";
import { Rotate } from "~/components/ui/animate/rotate";
import { SlideInOut } from "~/components/ui/animate/slide-in-out";
import { Button } from "~/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogHeader,
	DialogTitle,
	DialogTrigger,
} from "~/components/ui/dialog";
import { Form } from "~/components/ui/form";
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
import { useSlideInOut } from "~/hooks/animate/useSlideInOut";
import { useInitMessageStore } from "~/hooks/messages/useMessageStore";
import { usePagination } from "~/hooks/pagination/usePagination";
import { useAsync } from "~/hooks/useAsync";

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

	const { init, reset } = useInitMessageStore(threadId, { init: false });

	const invoke = useAsync(InvokeService.invokeAgent);

	const form = useForm({ defaultValues: { prompt: "" } });
	const { reset: formReset } = form;

	const [updateStatus, setUpdateStatus] = useState<{
		text: string;
		loading: boolean;
		error?: boolean;
	}>();

	const submitInvoke = form.handleSubmit(async ({ prompt }) => {
		if (!prompt) return;

		const targetedPrompt = `In the database table '${tableName}' do the following instruction:\n${prompt}`;

		setUpdateStatus({ text: "Sending Request to LLM", loading: true });

		formReset({ prompt: "" });

		const [error, data] = await invoke.executeAsync({
			slug: threadId,
			prompt: targetedPrompt,
			thread: threadId,
		});

		if (error) return setUpdateStatus({ text: error.message, loading: false });

		setUpdateStatus({ text: "LLM Processing Request", loading: true });

		let started = false;
		init(threadId, {
			onEvent: (event) => {
				if (event.runID !== data.runID) return;

				if (!started) {
					setUpdateStatus({ text: "Updating database table", loading: true });
					started = true;
				}

				if (event.error) {
					setUpdateStatus({
						text: event.error,
						loading: false,
						error: true,
					});
					return reset();
				}

				if (event.runComplete || event.error) {
					reset();

					setUpdateStatus({
						text: "Table updated, fetching changes",
						loading: true,
					});
					getTableRows.mutate().then(() =>
						setUpdateStatus({
							text: "Table updated successfully",
							loading: false,
						})
					);
				}
			},
		});
	});

	useEffect(() => {
		return () => {
			setUpdateStatus(undefined);
			formReset({ prompt: "" });
			reset();
		};
	}, [reset, open, formReset]);

	const [expandForm, setExpandForm] = useState(false);

	const trProps = useSlideInOut({ direction: "up" });

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

				<div className="flex flex-col gap-4">
					<div className="flex items-center justify-between gap-2">
						<Input
							placeholder="Quick Search"
							startContent={<SearchIcon />}
							onChange={(e) => tableStore.debouncedSearch(e.target.value)}
						/>

						<Button
							onClick={() => setExpandForm((prev) => !prev)}
							variant="ghost"
							endContent={
								<Rotate active={expandForm} degrees={180}>
									<ChevronDown />
								</Rotate>
							}
						>
							Update Table
						</Button>
					</div>

					<ExpandAndCollapse active={expandForm}>
						<Form {...form}>
							<form className="max-h-fit" onSubmit={submitInvoke}>
								<ControlledAutosizeTextarea
									control={form.control}
									name="prompt"
									placeholder="What do you want to change?"
									rows={1}
									minHeight={0}
									onKeyDown={(e) => {
										if (e.key === "Enter" && !e.shiftKey) {
											e.preventDefault();
											submitInvoke(e);
										}
									}}
									endContent={
										<Button
											type="submit"
											variant="ghost-primary"
											size="icon"
											shape="input-end"
											disabled={updateStatus?.loading}
										>
											<ArrowUpIcon />
										</Button>
									}
								/>
							</form>
						</Form>
					</ExpandAndCollapse>

					{updateStatus && (
						<AnimatePresence mode="wait">
							<SlideInOut
								direction={{ in: "up", out: "down" }}
								key={updateStatus.text}
								className={cn(
									"flex w-full items-center justify-end gap-2 text-muted-foreground",
									{
										"text-success":
											!updateStatus.error && !updateStatus.loading,
										"text-destructive": updateStatus.error,
									}
								)}
							>
								{updateStatus.error ? (
									<AlertCircleIcon className="size-4" />
								) : updateStatus.loading ? (
									<LoadingSpinner className="size-4" />
								) : (
									<CheckIcon className="size-4" />
								)}

								<small>{updateStatus.text}</small>
							</SlideInOut>
						</AnimatePresence>
					)}
				</div>

				<Table className="overflow-clip">
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
			<TableRow {...trProps} key={rowKey(row, index)}>
				{columns?.map((col) => (
					<TableCell key={col}>
						<p>{String(row[col] ?? "")}</p>
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
			<TableRow className="invisible border-transparent">
				<TableCell colSpan={columns?.length}>
					<p>.</p>
				</TableCell>
			</TableRow>
		);
	}

	function rowKey(row: Record<string, unknown>, index: number) {
		return `${index} ${Object.values(row).map(String).join("-")}`;
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
