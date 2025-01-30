import { GlobeIcon, GlobeLockIcon, ShieldOffIcon } from "lucide-react";

import { AssistantNamespace } from "~/lib/model/assistants";
import { ThreadsService } from "~/lib/service/api/threadsService";
import { ToolAuthApiService } from "~/lib/service/api/toolAuthApiService";

import { useToolReference } from "~/components/agent/ToolEntry";
import { ToolAuthenticationDialog } from "~/components/agent/shared/ToolAuthenticationDialog";
import { ConfirmationDialog } from "~/components/composed/ConfirmationDialog";
import { Button } from "~/components/ui/button";
import { DialogDescription } from "~/components/ui/dialog";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "~/components/ui/tooltip";
import { useConfirmationDialog } from "~/hooks/component-helpers/useConfirmationDialog";
import { useToolAuthPolling } from "~/hooks/toolAuth/useToolAuthPolling";
import { useAsync } from "~/hooks/useAsync";

type AgentAuthenticationProps = {
	tool: string;
	entityId: string;
	onUpdate: () => void;
	namespace: AssistantNamespace;
};

export function ToolAuthenticationStatus({
	tool,
	entityId,
	onUpdate,
	namespace,
}: AgentAuthenticationProps) {
	const authorize = useAsync(ToolAuthApiService.authenticateTools);
	const deauthorize = useAsync(ToolAuthApiService.deauthenticateTools);
	const cancelAuthorize = useAsync(ThreadsService.abortThread);

	const { threadId, reader } = authorize.data ?? {};

	const { toolInfo, setToolInfo } = useToolAuthPolling(namespace, entityId);

	const deleteConfirm = useConfirmationDialog();
	const authorizeConfirm = useConfirmationDialog();

	const handleAuthorize = () =>
		authorize.executeAsync(namespace, entityId, [tool]);

	const handleDeauthorize = async () => {
		if (!toolInfo) return;

		const [error] = await deauthorize.executeAsync(namespace, entityId, [tool]);

		if (error) return;

		setToolInfo({
			...Object.fromEntries(
				Object.entries(toolInfo).filter(([key]) => key !== tool)
			),
		});
		onUpdate();
	};

	const handleAuthorizeComplete = () => {
		if (!threadId) {
			console.error(new Error("Thread ID is undefined"));
			return;
		} else {
			reader?.cancel();
			cancelAuthorize.execute(threadId);
		}

		authorize.clear();

		if (!toolInfo) return;

		setToolInfo({
			...Object.fromEntries(
				Object.entries(toolInfo).filter(([key]) => key !== tool)
			),
		});
		onUpdate();
	};

	const loading =
		authorize.isLoading || cancelAuthorize.isLoading || !toolInfo?.[tool];

	const { authorized } = toolInfo?.[tool] ?? {};

	const { icon, label } = useToolReference(tool);

	const handleClick = () => {
		if (authorized) {
			deleteConfirm.interceptAsync(handleDeauthorize);
		} else {
			authorizeConfirm.interceptAsync(handleAuthorize);
		}
	};

	if (toolInfo?.[tool] && !toolInfo?.[tool].credentialNames?.length)
		return (
			<Tooltip>
				<TooltipTrigger asChild>
					<div>
						<Button size="icon" variant="ghost" disabled>
							<ShieldOffIcon />
						</Button>
					</div>
				</TooltipTrigger>

				<TooltipContent>
					<>This tool does not require authentication.</>
				</TooltipContent>
			</Tooltip>
		);

	return (
		<>
			<Tooltip>
				<TooltipContent className="max-w-xs">
					{loading ? (
						<>
							<b>Authentication Processing</b>
						</>
					) : authorized ? (
						<>
							<b>Global Auth Enabled: </b>
							{/* Leaving this here for now, will remove after we discuss the wording for this */}
							{/* Users will share the same account and will not be prompted to
                            login when using this tool. */}
							Users will not be prompted to use their own credentials to login,
							and will share the same global account when using this tool.
						</>
					) : (
						<>
							<b>Global Auth Disabled: </b>
							Users will be prompted to use their own credentials to login when
							using this tool.
						</>
					)}
				</TooltipContent>

				<TooltipTrigger asChild>
					<Button
						size="icon"
						variant="ghost"
						loading={loading}
						onClick={handleClick}
					>
						{authorized ? (
							<GlobeLockIcon className="text-success" />
						) : (
							<GlobeIcon />
						)}
					</Button>
				</TooltipTrigger>
			</Tooltip>

			<ConfirmationDialog
				{...authorizeConfirm.dialogProps}
				title={
					<span className="flex items-center gap-2">
						{icon}
						Pre-Authenticate {label}?
					</span>
				}
				content={
					<>
						<DialogDescription>
							{label} is currently not authenticated. Users will be prompted for
							authentication when using this tool in a new thread.
						</DialogDescription>

						<DialogDescription>
							You can pre-authenticate {label} to allow users to use this tool
							without authentication.
						</DialogDescription>
					</>
				}
				confirmProps={{
					children: `Authenticate Tool`,
					loading: authorize.isLoading,
					disabled: authorize.isLoading,
				}}
			/>

			<ToolAuthenticationDialog
				tool={tool}
				threadId={threadId}
				onComplete={handleAuthorizeComplete}
			/>

			<ConfirmationDialog
				{...deleteConfirm.dialogProps}
				title={
					<span className="flex items-center gap-2">
						<span>{icon}</span>
						<span>Remove Authentication?</span>
					</span>
				}
				description={`Are you sure you want to remove authentication for ${label}? this will require each thread to re-authenticate in order to use this tool.`}
				confirmProps={{
					variant: "destructive",
					children: "Delete Authentication",
				}}
			/>
		</>
	);
}
