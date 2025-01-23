import { useState } from "react";

import { OAuthProvider } from "~/lib/model/oauthApps/oauth-helpers";

import { CustomOauthAppDetail } from "~/components/oauth-apps/shared/CustomOauthAppDetail";
import { OAuthAppDetail } from "~/components/oauth-apps/shared/OAuthAppDetail";
import { Button } from "~/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from "~/components/ui/dialog";

type SelectToolAuthProps = {
	integration: string;
	configured: boolean;
	open: boolean;
	onOpenChange: (open: boolean) => void;
	onOAuthSelect: () => void;
	onPATSelect: () => void;
};

export function SelectToolAuth({
	integration,
	configured,
	open,
	onOpenChange,
	onOAuthSelect,
	onPATSelect,
}: SelectToolAuthProps) {
	const [showConfirmOauthForm, setShowConfirmOauthForm] = useState(false);
	const [openOauthDialog, setOpenOauthDialog] = useState(false);

	const isSpecedOauth =
		integration &&
		Object.values(OAuthProvider).includes(integration as OAuthProvider);

	const handleOAuthSelect = () => {
		if (configured) {
			onOAuthSelect();
		} else if (!configured && !isSpecedOauth) {
			setShowConfirmOauthForm(true);
		} else {
			setOpenOauthDialog(true);
		}
	};

	const handleOpenCustomOauthDialog = (open: boolean) => {
		if (!open) {
			setShowConfirmOauthForm(false);
		}

		setOpenOauthDialog(open);
		onOpenChange(open);
	};

	const handleOauthSuccess = () => {
		setOpenOauthDialog(false);
		onOAuthSelect();
	};

	return (
		<>
			<Dialog open={open} onOpenChange={onOpenChange}>
				<DialogContent>
					<DialogHeader>
						<DialogTitle>
							{showConfirmOauthForm
								? "Confirm OAuth Method"
								: "Authentication Method"}
						</DialogTitle>
					</DialogHeader>
					<DialogDescription>
						{showConfirmOauthForm
							? "In order to use the OAuth for this tool, you will need to set it up! Would you like to set it up now?"
							: "This tool has personal access token (PAT) and OAuth support. Select the authentication method you would like to use for this tool."}
					</DialogDescription>
					<div className="flex flex-col gap-2">
						{showConfirmOauthForm ? (
							<>
								<Button onClick={() => setOpenOauthDialog(true)}>
									Configure OAuth
								</Button>
								<Button
									onClick={() => {
										setShowConfirmOauthForm(false);
										onPATSelect();
									}}
									variant="secondary"
								>
									Use Personal Access Token (PAT)
								</Button>
							</>
						) : (
							<>
								<Button onClick={handleOAuthSelect}>OAuth</Button>
								<Button onClick={onPATSelect}>
									Personal Access Token (PAT)
								</Button>
							</>
						)}
					</div>
				</DialogContent>
			</Dialog>
			{isSpecedOauth ? (
				<OAuthAppDetail
					open={openOauthDialog}
					onOpenChange={setOpenOauthDialog}
					onSuccess={handleOauthSuccess}
					type={integration as OAuthProvider}
				/>
			) : (
				<CustomOauthAppDetail
					open={openOauthDialog}
					onOpenChange={handleOpenCustomOauthDialog}
					integration={integration}
				/>
			)}
		</>
	);
}
