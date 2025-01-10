import { OAuthAppDetail } from "~/lib/model/oauthApps";

import { CustomOAuthAppForm } from "~/components/oauth-apps/CustomOAuthAppForm";
import {
	Dialog,
	DialogContent,
	DialogHeader,
	DialogTitle,
} from "~/components/ui/dialog";

export function CustomOauthAppDetail({
	open,
	spec,
	onOpenChange,
	type,
}: {
	open: boolean;
	spec?: OAuthAppDetail;
	onOpenChange: (open: boolean) => void;
	type?: string;
}) {
	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>Configure Custom OAuth</DialogTitle>
				</DialogHeader>
				<CustomOAuthAppForm
					defaultData={spec?.appOverride}
					integration={type}
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
