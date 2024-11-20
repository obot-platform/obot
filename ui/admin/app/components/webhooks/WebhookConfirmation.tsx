import { $path } from "remix-routes";

import { Webhook } from "~/lib/model/webhooks";

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
    token?: string;
    secret: string;
    type?: "github";
};

export const WebhookConfirmation = ({
    webhook,
    token: _token,
    secret,
    type: _ = "github",
}: WebhookConfirmationProps) => {
    return (
        <Dialog open>
            <DialogContent className="max-w-[700px]">
                <DialogHeader>
                    <DialogTitle>Webhook Saved</DialogTitle>
                </DialogHeader>

                <DialogDescription>
                    Your webhook has been saved in Otto. Make sure to copy
                    payload url and secret to your webhook provider.
                </DialogDescription>

                <DialogDescription>
                    This information will not be shown again.
                </DialogDescription>

                <div className="flex items-center justify-between">
                    <TypographyP>Payload URL: </TypographyP>
                    <CopyText
                        text={webhook.links?.invoke ?? ""}
                        className="min-w-fit"
                    />
                </div>

                <div className="flex items-center justify-between">
                    <TypographyP>Secret: </TypographyP>
                    <CopyText
                        className="min-w-fit"
                        displayText={secret}
                        text={secret ?? ""}
                    />
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
