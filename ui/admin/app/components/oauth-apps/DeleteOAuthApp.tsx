import { toast } from "sonner";
import { mutate } from "swr";

import { OAuthApp } from "~/lib/model/oauthApps";
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
	app,
	disableTooltip,
}: {
	app: OAuthApp;
	disableTooltip?: boolean;
}) {
	const deleteOAuthApp = useAsync(async () => {
		await OauthAppService.deleteOauthApp(app.id);
		await mutate(OauthAppService.getOauthApps.key());

		toast.success(`${app.name} OAuth configuration deleted`);
	});

	const title = `Delete ${app.name} OAuth`;

	const description = `By clicking \`Delete\`, you will delete your ${app.name} OAuth configuration.`;
	const buttonText = `Delete ${app.name} OAuth`;

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
