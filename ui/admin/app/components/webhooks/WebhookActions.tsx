import { EllipsisIcon } from "lucide-react";
import { $path } from "safe-routes";

import { Webhook } from "~/lib/model/webhooks";

import { Button } from "~/components/ui/button";
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuGroup,
    DropdownMenuItem,
    DropdownMenuTrigger,
} from "~/components/ui/dropdown-menu";
import { Link } from "~/components/ui/link";
import { DeleteWebhook } from "~/components/webhooks/DeleteWebhook";

export function WebhookActions({ webhook }: { webhook: Webhook }) {
    return (
        <DropdownMenu>
            <DropdownMenuTrigger asChild>
                <Button
                    variant="ghost"
                    size="icon"
                    onClick={(e) => e.stopPropagation()}
                >
                    <EllipsisIcon />
                </Button>
            </DropdownMenuTrigger>

            <DropdownMenuContent
                className="w-48 p-2 flex flex-col gap-1"
                side="bottom"
                align="end"
                onClick={(e) => e.stopPropagation()}
            >
                <DropdownMenuGroup>
                    <Link
                        to={$path("/webhooks/:webhook", {
                            webhook: webhook.id,
                        })}
                        as="div"
                    >
                        <DropdownMenuItem>Edit</DropdownMenuItem>
                    </Link>

                    <DeleteWebhook id={webhook.id}>
                        <DropdownMenuItem onSelect={(e) => e.preventDefault()}>
                            Delete
                        </DropdownMenuItem>
                    </DeleteWebhook>
                </DropdownMenuGroup>
            </DropdownMenuContent>
        </DropdownMenu>
    );
}
