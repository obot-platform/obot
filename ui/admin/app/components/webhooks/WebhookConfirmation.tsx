import { useEffect, useState } from "react";
import { $path } from "safe-routes";
import useSWR from "swr";

import { Webhook } from "~/lib/model/webhooks";
import { WebhookApiService } from "~/lib/service/api/webhookApiService";
import { cn } from "~/lib/utils";

import { CopyText } from "~/components/composed/CopyText";
import { LoadingSpinner } from "~/components/ui/LoadingSpinner";
import { Button } from "~/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "~/components/ui/dialog";
import { Link } from "~/components/ui/link";

export type WebhookConfirmationProps = {
	webhook: Webhook;
	original?: Webhook;
	token?: string;
	secret: string;
	type?: "github";
	tokenRemoved: boolean;
	secretRemoved: boolean;
	onContinue?: () => void;
};

export const WebhookConfirmation = ({
	webhook,
	original: _original,
	token,
	secret,
	type: _type = "github",
	tokenRemoved: _tokenRemoved,
	secretRemoved,
	onContinue,
}: WebhookConfirmationProps) => {
	// note(ryanhopperlowe): commenting out as to not lose the logic here
	// const showUrlChange =
	//     !original ||
	//     original.links?.invoke !== webhook.links?.invoke ||
	//     !!token ||
	//     tokenRemoved;

	const [blockPollingWebhook, setBlockPollingWebhook] = useState(false);
	const getWebhookData = useSWR(
		WebhookApiService.getWebhookById.key(webhook.id),
		() => WebhookApiService.getWebhookById(webhook.id),
		{
			refreshInterval: blockPollingWebhook ? undefined : 1000,
		}
	);

	const webhookData = getWebhookData.data ?? webhook;

	useEffect(() => {
		if (webhookData?.alias && webhookData.aliasAssigned === undefined) {
			setBlockPollingWebhook(false);
		} else {
			setBlockPollingWebhook(true);
		}
	}, [webhookData]);

	return (
		<Dialog open>
			<DialogContent className="max-w-[700px]" hideCloseButton>
				<DialogHeader>
					<DialogTitle>Webhook Saved</DialogTitle>
				</DialogHeader>

				<DialogDescription>
					Your webhook has been saved in Obot. Make sure to copy the payload URL
					and secret to your webhook provider.
				</DialogDescription>

				<DialogDescription>
					This information will not be shown again.
				</DialogDescription>

				<div className={cn("flex flex-col gap-1")}>
					<p>Payload URL: </p>
					{webhookData?.alias && webhookData.aliasAssigned === undefined ? (
						<div className="flex items-center gap-2">
							<LoadingSpinner className="h-4 w-4" />
						</div>
					) : (
						<CopyText
							text={getWebhookUrl(webhookData, token)}
							className="w-fit-content max-w-full"
						/>
					)}
				</div>

				<div
					className={cn("flex flex-col gap-1", {
						"flex-row gap-2": !secret,
					})}
				>
					<p>Secret: </p>
					{secret ? (
						<CopyText
							className="min-w-fit"
							displayText={secret}
							text={secret ?? ""}
						/>
					) : (
						<p className="text-muted-foreground">
							({secretRemoved ? "None" : "Unchanged"})
						</p>
					)}
				</div>

				<DialogFooter>
					{onContinue ? (
						<Button onClick={onContinue}>Continue</Button>
					) : (
						<Link as="button" className="w-full" to={$path("/tasks")}>
							Continue
						</Link>
					)}
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};

function getWebhookUrl(webhook: Webhook, token?: string) {
	if (!token) return webhook.links?.invoke ?? "";

	const url = new URL(webhook.links?.invoke ?? "");
	url.searchParams.set("token", token);

	return url.toString();
}
