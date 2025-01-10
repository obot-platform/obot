import { SettingsIcon, TriangleAlertIcon } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { mutate } from "swr";

import { OAuthProvider } from "~/lib/model/oauthApps/oauth-helpers";
import { ToolReference } from "~/lib/model/toolReferences";
import { ToolReferenceService } from "~/lib/service/api/toolreferenceService";

import { ConfirmationDialog } from "~/components/composed/ConfirmationDialog";
import { CustomOauthAppDetail } from "~/components/oauth-apps/CustomOauthAppDetail";
import { OAuthAppDetail } from "~/components/oauth-apps/OAuthAppDetail";
import { Button } from "~/components/ui/button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "~/components/ui/dropdown-menu";
import { useConfirmationDialog } from "~/hooks/component-helpers/useConfirmationDialog";
import { useOAuthAppList } from "~/hooks/oauthApps/useOAuthApps";
import { useAsync } from "~/hooks/useAsync";

type ToolDropdownProps = {
	tool: ToolReference;
};

export function ToolDropdown({ tool }: ToolDropdownProps) {
	const [configureAuthOpen, setConfigureAuthOpen] = useState(false);
	const { dialogProps, interceptAsync } = useConfirmationDialog();

	const oauthApps = useOAuthAppList();
	const oauthAppsMap = new Map(
		oauthApps.map((app) => [app.appOverride?.integration ?? app.type, app])
	);
	const oauth = oauthAppsMap.get(tool?.metadata?.oauth ?? "");

	const deleteTool = useAsync(ToolReferenceService.deleteToolReference, {
		onSuccess: () => {
			toast.success("Tool has been deleted.");
			mutate(ToolReferenceService.getToolReferencesCategoryMap.key("tool"));
		},
		onError: () => {
			toast.error("Something went wrong");
		},
	});

	const handleDelete = () =>
		interceptAsync(() => deleteTool.executeAsync(tool.id));

	const toolOauthMetadata = tool.metadata?.oauth;

	const isCustomOauth =
		toolOauthMetadata && (!oauth || oauth?.type === "custom");
	const requiresConfiguration =
		toolOauthMetadata &&
		(!oauth || (oauth && oauth.noGatewayIntegration && !oauth.appOverride));
	return (
		<>
			<DropdownMenu>
				<DropdownMenuTrigger asChild>
					<Button variant="ghost" size="icon-sm">
						{requiresConfiguration ? (
							<TriangleAlertIcon className="text-warning" />
						) : (
							<SettingsIcon />
						)}
					</Button>
				</DropdownMenuTrigger>
				<DropdownMenuContent className="w-56" side="bottom" align="start">
					{!tool.error && toolOauthMetadata && (
						<DropdownMenuItem
							className="flex items-center gap-1"
							onClick={() => {
								setConfigureAuthOpen(true);
							}}
						>
							{requiresConfiguration && (
								<TriangleAlertIcon className="h-4 w-4 text-warning" />
							)}
							Configure OAuth
						</DropdownMenuItem>
					)}
					{!tool.builtin && (
						<DropdownMenuItem
							className="text-destructive"
							onClick={handleDelete}
						>
							Delete Tool
						</DropdownMenuItem>
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
			{toolOauthMetadata ? (
				isCustomOauth ? (
					<CustomOauthAppDetail
						open={configureAuthOpen}
						onOpenChange={setConfigureAuthOpen}
						spec={oauth}
						type={toolOauthMetadata}
					/>
				) : (
					<OAuthAppDetail
						open={configureAuthOpen}
						onOpenChange={setConfigureAuthOpen}
						type={toolOauthMetadata as OAuthProvider}
					/>
				)
			) : null}
		</>
	);
}
