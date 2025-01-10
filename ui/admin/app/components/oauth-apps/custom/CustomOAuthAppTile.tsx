import { Trash } from "lucide-react";
import { mutate } from "swr";

import { OAuthApp } from "~/lib/model/oauthApps";
import { OauthAppService } from "~/lib/service/api/oauthAppService";
import { cn, timeSince } from "~/lib/utils";

import { ConfirmationDialog } from "~/components/composed/ConfirmationDialog";
import { EditCustomOAuthApp } from "~/components/oauth-apps/custom/EditCustomOAuthApp";
import { Button } from "~/components/ui/button";
import {
	Card,
	CardContent,
	CardFooter,
	CardHeader,
} from "~/components/ui/card";
import { useAsync } from "~/hooks/useAsync";

type CustomOAuthAppTileProps = {
	app: OAuthApp;
};

export function CustomOAuthAppTile({ app }: CustomOAuthAppTileProps) {
	const deleteApp = useAsync(OauthAppService.deleteOauthApp, {
		onSuccess: () => {
			mutate(OauthAppService.getOauthApps.key());
		},
	});

	return (
		<Card className={cn("border-2 border-primary")}>
			<CardHeader className="flex flex-row justify-between">
				<div className="flex items-center gap-2">
					<h3>{app.name}</h3>
				</div>
			</CardHeader>

			<CardContent>
				<p className="truncate">{app.integration}</p>
			</CardContent>

			<CardFooter className="flex flex-grow items-center justify-between">
				<small className="text-muted-foreground">
					{timeSince(new Date(app.created))} ago
				</small>

				<div className="flex items-center gap-2">
					<EditCustomOAuthApp app={app} />
					<ConfirmationDialog
						title="Delete OAuth App"
						description="Are you sure you want to delete this OAuth app? This action cannot be undone."
						onConfirm={() => deleteApp.execute(app.id)}
						confirmProps={{
							variant: "destructive",
							children: "Delete",
						}}
					>
						<Button
							variant="ghost"
							size="icon"
							className="h-8 w-8 p-0"
							disabled={deleteApp.isLoading}
							loading={deleteApp.isLoading}
						>
							<Trash />
						</Button>
					</ConfirmationDialog>
				</div>
			</CardFooter>
		</Card>
	);
}
