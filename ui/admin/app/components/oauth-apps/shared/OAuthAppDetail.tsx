import { useState } from "react";

import { OAuthApp, OAuthAppSpecMap } from "~/lib/model/oauthApps";
import {
	OAuthAppSpec,
	OAuthProvider,
} from "~/lib/model/oauthApps/oauth-helpers";

import { ConfigureOAuthApp } from "~/components/oauth-apps/ConfigureOAuthApp";
import { DeleteOAuthApp } from "~/components/oauth-apps/DeleteOAuthApp";
import { OAuthAppTypeIcon } from "~/components/oauth-apps/OAuthAppTypeIcon";
import { Button } from "~/components/ui/button";
import {
	Dialog,
	DialogClose,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "~/components/ui/dialog";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "~/components/ui/tooltip";
import { useOAuthAppInfo } from "~/hooks/oauthApps/useOAuthApps";

export function OAuthAppDetail({
	type,
	open,
	onOpenChange,
	onSuccess,
}: {
	type: OAuthProvider;
	open: boolean;
	onOpenChange: (open: boolean) => void;
	onSuccess?: () => void;
}) {
	const [successModalOpen, setSuccessModalOpen] = useState(false);

	const oAuthApp = useOAuthAppInfo(type);

	const spec = type !== "custom" ? OAuthAppSpecMap[type] : null;
	if (!spec) {
		console.error(`Custom OAuth app should not be used with OAuthAppDetail.`);
		return null;
	}

	const handleSuccess = () => {
		setSuccessModalOpen(true);
		onSuccess?.();
	};

	return (
		<>
			<Dialog open={open} onOpenChange={onOpenChange}>
				<DialogDescription hidden>OAuth App Details</DialogDescription>

				<DialogContent>
					<DialogHeader>
						<DialogTitle className="flex items-center gap-2">
							<OAuthAppTypeIcon type={type} />

							<span>{spec.displayName}</span>
						</DialogTitle>
					</DialogHeader>

					{oAuthApp ? (
						<Content app={oAuthApp} spec={spec} onSuccess={handleSuccess} />
					) : (
						<EmptyContent spec={spec} onSuccess={handleSuccess} />
					)}
				</DialogContent>
			</Dialog>

			<Dialog open={successModalOpen} onOpenChange={setSuccessModalOpen}>
				<DialogContent>
					<DialogTitle>
						Successfully Configured {spec.displayName} OAuth App
					</DialogTitle>

					<DialogDescription>
						Obot will now use your custom {spec.displayName} OAuth app to
						authenticate users.
					</DialogDescription>

					<DialogFooter>
						<DialogClose asChild>
							<Button className="w-full">Close</Button>
						</DialogClose>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</>
	);
}

function EmptyContent({
	spec,
	onSuccess,
}: {
	spec: OAuthAppSpec;
	onSuccess: () => void;
}) {
	return (
		<div className="flex flex-col gap-2">
			<p>
				{spec.displayName} OAuth is not configured. You must configure it to
				enable tools that interact with protected {spec.displayName} APIs.
			</p>

			<p className="mb-4">
				You can also configure {spec.displayName} OAuth by clicking the button
				below.
			</p>

			<ConfigureOAuthApp spec={spec} onSuccess={onSuccess} />
		</div>
	);
}

function Content({
	app,
	onSuccess,
	spec,
}: {
	app: OAuthApp;
	onSuccess: () => void;
	spec: OAuthAppSpec;
}) {
	return (
		<div className="flex flex-col gap-2">
			<p>
				Obot only supports one custom {app.name} OAuth. If you need to use a
				different configuration, you can replace the current configuration with
				a new one.
			</p>

			<p>When {app.name} OAuth is used, Obot will use your custom OAuth app.</p>

			<div className="grid grid-cols-2 gap-2 px-8 py-4">
				<p>
					<strong>Client ID</strong>
				</p>

				<Tooltip>
					<TooltipTrigger className="truncate underline decoration-dotted">
						{app.clientID}
					</TooltipTrigger>

					<TooltipContent>{app.clientID}</TooltipContent>
				</Tooltip>

				<p>
					<strong>Client Secret</strong>
				</p>
				<p>****************</p>
			</div>

			<ConfigureOAuthApp app={app} spec={spec} onSuccess={onSuccess} />
			<DeleteOAuthApp app={app} disableTooltip />
		</div>
	);
}
