import { $path } from "safe-routes";

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
    original: _original,
    token,
    secret,
    type: _type = "github",
    tokenRemoved: _tokenRemoved,
    secretRemoved,
}: WebhookConfirmationProps) => {
    // note(ryanhopperlowe): commenting out as to not lose the logic here
    // const showUrlChange =
    //     !original ||
    //     original.links?.invoke !== webhook.links?.invoke ||
    //     !!token ||
    //     tokenRemoved;

    return (
        <Dialog open>
            <DialogContent className="max-w-[700px]" hideCloseButton>
                <DialogHeader>
                    <DialogTitle>Webhook Saved</DialogTitle>
                </DialogHeader>

                <DialogDescription>
                    Your webhook has been saved in Obot. Make sure to copy the
                    payload URL and secret to your webhook provider.
                </DialogDescription>

                <DialogDescription>
                    This information will not be shown again.
                </DialogDescription>

                <div className={cn("flex flex-col gap-1")}>
                    <TypographyP>Payload URL: </TypographyP>
                    <CopyText
                        text={getWebhookUrl(webhook, token)}
                        className="w-fit-content max-w-full"
                    />
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
