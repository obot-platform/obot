import { SettingsIcon, TriangleAlertIcon } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { mutate } from "swr";

import { ToolReference } from "~/lib/model/toolReferences";
import { ToolReferenceService } from "~/lib/service/api/toolreferenceService";
import { cn } from "~/lib/utils";

import { ConfirmationDialog } from "~/components/composed/ConfirmationDialog";
import { ToolOauthConfig } from "~/components/tools/ToolOauthConfig";
import { LoadingSpinner } from "~/components/ui/LoadingSpinner";
import { Button } from "~/components/ui/button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "~/components/ui/dropdown-menu";
import { useConfirmationDialog } from "~/hooks/component-helpers/useConfirmationDialog";
import { useOauthAppMap } from "~/hooks/oauthApps/useOAuthApps";
import { useAsync } from "~/hooks/useAsync";
import { usePollSingleTool } from "~/hooks/usePollSingleTool";

export function ToolCardActions({ tool }: { tool: ToolReference }) {
	const [configureAuthOpen, setConfigureAuthOpen] = useState(false);
	const { dialogProps, interceptAsync } = useConfirmationDialog();

	const oauthAppsMap = useOauthAppMap();
	const oauth = oauthAppsMap.get(tool?.metadata?.oauth ?? "");

	const deleteTool = useAsync(ToolReferenceService.deleteToolReference, {
		onSuccess: () => {
			toast.success("Tool has been deleted.");
			mutate(ToolReferenceService.getToolReferences.key("tool"));
		},
		onError: () => {
			toast.error("Something went wrong");
		},
	});

	const { startPolling, isPolling } = usePollSingleTool(tool.id);

	const forceRefresh = useAsync(
		ToolReferenceService.forceRefreshToolReference,
		{
			onSuccess: () => {
				toast.success("Tool reference force refreshed");
				startPolling();
			},
		}
	);

	const handleDelete = () =>
		interceptAsync(() => deleteTool.executeAsync(tool.id));

	const toolOauthMetadata = tool.metadata?.oauth;
	const requiresConfiguration = toolOauthMetadata && !oauth;

	if (tool.builtin && !toolOauthMetadata) return null;
	return (
		<>
			<DropdownMenu>
				<DropdownMenuTrigger asChild>
					<Button variant="ghost" size="icon-sm">
						{forceRefresh.isLoading || isPolling ? (
							<LoadingSpinner />
						) : requiresConfiguration ? (
							<TriangleAlertIcon className="text-warning" />
						) : (
							<SettingsIcon />
						)}
					</Button>
				</DropdownMenuTrigger>
				<DropdownMenuContent className="w-56" side="top" align="start">
					{!tool.error && toolOauthMetadata && (
						<DropdownMenuItem
							className={cn("flex items-center gap-1", {
								"text-warning": requiresConfiguration,
							})}
							onClick={() => setConfigureAuthOpen(true)}
						>
							{requiresConfiguration && (
								<TriangleAlertIcon className="h-4 w-4 text-warning" />
							)}
							Configure OAuth
						</DropdownMenuItem>
					)}
					{!tool.builtin && (
						<>
							<DropdownMenuItem onClick={() => forceRefresh.execute(tool.id)}>
								Refresh Tool
							</DropdownMenuItem>
							<DropdownMenuItem
								className="text-destructive"
								onClick={handleDelete}
							>
								Delete Tool
							</DropdownMenuItem>
						</>
					)}
				</DropdownMenuContent>
			</DropdownMenu>
			<ConfirmationDialog
				{...dialogProps}
				title="Delete Tool"
				description="Are you sure you want to delete this tool? This action cannot be undone."
				confirmProps={{
					variant: "destructive",
					loading: deleteTool.isLoading,
					disabled: deleteTool.isLoading,
				}}
			/>
			<ToolOauthConfig
				tool={tool}
				open={configureAuthOpen}
				onOpenChange={setConfigureAuthOpen}
			/>
		</>
	);
}
