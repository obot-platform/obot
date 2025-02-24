import { mutate } from "swr";

import { OauthAppService } from "~/lib/service/api/oauthAppService";

import { ConfirmationDialog } from "~/components/composed/ConfirmationDialog";
import { LoadingSpinner } from "~/components/ui/LoadingSpinner";
import { Button } from "~/components/ui/button";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "~/components/ui/tooltip";
import { useAsync } from "~/hooks/useAsync";

export function DeleteOAuthApp({
	appId,
	disableTooltip,
	name,
}: {
	appId: string;
	disableTooltip?: boolean;
	name: string;
}) {
	const deleteOAuthApp = useAsync(async () => {
		await OauthAppService.deleteOauthApp(appId);
		await mutate(OauthAppService.getOauthApps.key());
	});

	const title = `Delete ${name} OAuth`;

	const description = `By clicking \`Delete\`, you will delete your ${name} OAuth configuration.`;
	const buttonText = `Delete ${name} OAuth`;

	return (
		<div className="flex gap-2">
			<Tooltip open={getIsOpen()}>
				<ConfirmationDialog
					title={title}
					description={description}
					onConfirm={deleteOAuthApp.execute}
					confirmProps={{
						variant: "destructive",
						children: buttonText,
					}}
				>
					<TooltipTrigger asChild>
						<Button
							variant="destructive"
							className="w-full"
							disabled={deleteOAuthApp.isLoading}
						>
							{deleteOAuthApp.isLoading ? (
								<LoadingSpinner className="mr-2 h-4 w-4" />
							) : null}
							{buttonText}
						</Button>
					</TooltipTrigger>
				</ConfirmationDialog>

				<TooltipContent>Delete</TooltipContent>
			</Tooltip>
		</div>
	);

	function getIsOpen() {
		if (disableTooltip) return false;
	}
}
