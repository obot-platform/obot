import { PlusIcon, SearchIcon } from "lucide-react";
import { useMemo, useState } from "react";
import { MetaFunction } from "react-router";
import useSWR, { preload } from "swr";

import { convertToolReferencesToCategoryMap } from "~/lib/model/toolReferences";
import { ToolReferenceService } from "~/lib/service/api/toolreferenceService";
import { RouteHandle } from "~/lib/service/routeHandles";

import { ErrorDialog } from "~/components/composed/ErrorDialog";
import { CreateTool } from "~/components/tools/CreateTool";
import { ToolGrid } from "~/components/tools/toolGrid";
import { Button } from "~/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
	DialogTrigger,
} from "~/components/ui/dialog";
import { Input } from "~/components/ui/input";
import { ScrollArea } from "~/components/ui/scroll-area";

export async function clientLoader() {
	await Promise.all([
		preload(ToolReferenceService.getToolReferences.key("tool"), () =>
			ToolReferenceService.getToolReferences("tool")
		),
	]);
	return null;
}

export default function Tools() {
	const getTools = useSWR(
		ToolReferenceService.getToolReferences.key("tool"),
		() => ToolReferenceService.getToolReferences("tool"),
		{ fallbackData: [] }
	);

	const toolCategories = useMemo(
		() => convertToolReferencesToCategoryMap(getTools.data),
		[getTools.data]
	);

	const [isDialogOpen, setIsDialogOpen] = useState(false);
	const [searchQuery, setSearchQuery] = useState("");
	const [errorDialogError, setErrorDialogError] = useState("");

	const handleCreateSuccess = () => {
		getTools.mutate();
		setIsDialogOpen(false);
	};

	const handleDelete = async (id: string) => {
		await ToolReferenceService.deleteToolReference(id);
		getTools.mutate();
	};

	const handleErrorDialogError = (error: string) => {
		getTools.mutate();
		setErrorDialogError(error);
		setIsDialogOpen(false);
	};

	return (
		<ScrollArea className="flex h-full flex-col gap-4 p-8">
			<div className="flex items-center justify-between">
				<h2>Tools</h2>
				<div className="flex items-center space-x-2">
					<div className="relative">
						<SearchIcon className="absolute left-3 top-1/2 h-5 w-5 -translate-y-1/2 transform text-gray-400" />
						<Input
							type="text"
							placeholder="Search for tools..."
							value={searchQuery}
							onChange={(e) => setSearchQuery(e.target.value)}
							className="w-64 pl-10"
						/>
					</div>
					<Dialog open={isDialogOpen} onOpenChange={setIsDialogOpen}>
						<DialogTrigger asChild>
							<Button variant="outline">
								<PlusIcon className="mr-2 h-4 w-4" />
								Register New Tool
							</Button>
						</DialogTrigger>
						<DialogContent className="max-w-2xl">
							<DialogHeader>
								<DialogTitle>Create New Tool Reference</DialogTitle>
								<DialogDescription>
									Register a new tool reference to use in your agents.
								</DialogDescription>
							</DialogHeader>
							<CreateTool
								onError={handleErrorDialogError}
								onSuccess={handleCreateSuccess}
							/>
						</DialogContent>
					</Dialog>
					<ErrorDialog
						error={errorDialogError}
						isOpen={errorDialogError !== ""}
						onClose={() => setErrorDialogError("")}
					/>
				</div>
			</div>

			{toolCategories && (
				<ToolGrid
					toolCategories={toolCategories}
					filter={searchQuery}
					onDelete={handleDelete}
				/>
			)}
		</ScrollArea>
	);
}

export const handle: RouteHandle = {
	breadcrumb: () => [{ content: "Tools" }],
};

export const meta: MetaFunction = () => {
	return [{ title: `Obot • Tools` }];
};
