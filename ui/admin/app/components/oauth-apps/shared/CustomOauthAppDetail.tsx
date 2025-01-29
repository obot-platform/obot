import { OAuthApp } from "~/lib/model/oauthApps";

import { CustomOAuthAppForm } from "~/components/oauth-apps/CustomOAuthAppForm";
import {
	Dialog,
	DialogContent,
	DialogHeader,
	DialogTitle,
} from "~/components/ui/dialog";

export function CustomOauthAppDetail({
	open,
	app,
	onOpenChange,
	alias,
}: {
	open: boolean;
	app?: OAuthApp;
	onOpenChange: (open: boolean) => void;
	alias?: string;
}) {
	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>Configure Custom OAuth</DialogTitle>
				</DialogHeader>
				<CustomOAuthAppForm
					defaultData={app}
					alias={alias}
					onComplete={() => {
						onOpenChange(false);
					}}
					onCancel={() => {
						onOpenChange(false);
					}}
				/>
			</DialogContent>
		</Dialog>
	);
}
