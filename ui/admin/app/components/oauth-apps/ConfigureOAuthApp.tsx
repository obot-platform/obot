import { SettingsIcon } from "lucide-react";
import { toast } from "sonner";
import { mutate } from "swr";

import { OAuthApp, OAuthAppParams } from "~/lib/model/oauthApps";
import { OAuthAppSpec } from "~/lib/model/oauthApps/oauth-helpers";
import { OauthAppService } from "~/lib/service/api/oauthAppService";

import { OAuthAppForm } from "~/components/oauth-apps/OAuthAppForm";
import { OAuthAppTypeIcon } from "~/components/oauth-apps/OAuthAppTypeIcon";
import { Button } from "~/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogTitle,
	DialogTrigger,
} from "~/components/ui/dialog";
import { ScrollArea } from "~/components/ui/scroll-area";
import { useAsync } from "~/hooks/useAsync";
import { useDisclosure } from "~/hooks/useDisclosure";

export function ConfigureOAuthApp({
	app,
	onSuccess,
	spec,
}: {
	app?: OAuthApp;
	spec: OAuthAppSpec;
	onSuccess: () => void;
}) {
	const modal = useDisclosure();

	const createApp = useAsync(async (data: OAuthAppParams) => {
		await OauthAppService.createOauthApp({
			...data,
			type: spec.type,
			integration: spec.type,
		});

		await mutate(OauthAppService.getOauthApps.key());

		modal.onClose();
		toast.success(`${spec.displayName} OAuth configuration created`);
		onSuccess();
	});

	const updateApp = useAsync(async (data: OAuthAppParams) => {
		if (!app) return;
		await OauthAppService.updateOauthApp(app.id, {
			...data,
			type: app.type,
			integration: app.integration,
		});

		await mutate(OauthAppService.getOauthApps.key());

		modal.onClose();
		toast.success(`${app.name} OAuth configuration updated`);
		onSuccess();
	});

	const editLabel = app
		? "Replace Configuration"
		: `Configure ${spec?.displayName} OAuth App`;
	return (
		<Dialog open={modal.isOpen} onOpenChange={modal.onOpenChange}>
			<DialogTrigger asChild>
				<Button className="w-full">
					<SettingsIcon className="mr-2 h-4 w-4" />
					{editLabel}
				</Button>
			</DialogTrigger>

			<DialogContent
				className="lg:max-w-3xl"
				classNames={{
					overlay: "opacity-0",
				}}
				aria-describedby="create-oauth-app"
			>
				<DialogTitle className="flex items-center gap-2 px-4">
					<OAuthAppTypeIcon type={app?.type || spec.type} />
					Configure {app?.name || spec.displayName} OAuth App
				</DialogTitle>

				<DialogDescription hidden>
					Create a new OAuth app for {app?.name || spec?.displayName}
				</DialogDescription>

				<ScrollArea className="max-h-[80vh] px-4">
					<OAuthAppForm
						spec={spec}
						onSubmit={app ? updateApp.execute : createApp.execute}
						isLoading={app ? updateApp.isLoading : createApp.isLoading}
					/>
				</ScrollArea>
			</DialogContent>
		</Dialog>
	);
}
