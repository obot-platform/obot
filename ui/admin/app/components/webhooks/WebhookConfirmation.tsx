import { $path } from "remix-routes";

import { Webhook } from "~/lib/model/webhooks";
import { cn } from "~/lib/utils";

import { TypographyP } from "~/components/Typography";
import { CopyText } from "~/components/composed/CopyText";
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
};

export const WebhookConfirmation = ({
    webhook,
    original,
    token,
    secret,
    type: _ = "github",
    tokenRemoved,
    secretRemoved,
}: WebhookConfirmationProps) => {
    const showUrlChange =
        !original ||
        original.links?.invoke !== webhook.links?.invoke ||
        !!token ||
        tokenRemoved;

    return (
        <Dialog open>
            <DialogContent className="max-w-[700px]" hideCloseButton>
                <DialogHeader>
                    <DialogTitle>Webhook Saved</DialogTitle>
                </DialogHeader>

                <DialogDescription>
                    Your webhook has been saved in Otto8. Make sure to copy the
                    payload URL and secret to your webhook provider.
                </DialogDescription>

                <DialogDescription>
                    This information will not be shown again.
                </DialogDescription>

                <div
                    className={cn("flex flex-col gap-1", {
                        "flex-row gap-2": !showUrlChange,
                    })}
                >
                    <TypographyP>Payload URL: </TypographyP>
                    {showUrlChange ? (
                        <CopyText
                            text={getWebhookUrl(webhook, token)}
                            className="min-w-fit"
                        />
                    ) : (
                        <TypographyP className="text-muted-foreground">
                            (Unchanged)
                        </TypographyP>
                    )}
                </div>

                <div
                    className={cn("flex flex-col gap-1", {
                        "flex-row gap-2": !secret,
                    })}
                >
                    <TypographyP>Secret: </TypographyP>
                    {secret ? (
                        <CopyText
                            className="min-w-fit"
                            displayText={secret}
                            text={secret ?? ""}
                        />
                    ) : (
                        <TypographyP className="text-muted-foreground">
                            ({secretRemoved ? "None" : "Unchanged"})
                        </TypographyP>
                    )}
                </div>

                <DialogFooter>
                    <Link
                        as="button"
                        className="w-full"
                        to={$path("/webhooks")}
                    >
                        Continue
                    </Link>
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
