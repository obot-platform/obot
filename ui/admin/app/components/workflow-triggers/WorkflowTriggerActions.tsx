import { EllipsisIcon } from "lucide-react";
import { $path } from "safe-routes";

import { WorkflowTrigger } from "~/lib/model/workflow-trigger";

import { Button } from "~/components/ui/button";
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuGroup,
    DropdownMenuItem,
    DropdownMenuTrigger,
} from "~/components/ui/dropdown-menu";
import { Link } from "~/components/ui/link";
import { DeleteWorkflowTrigger } from "~/components/workflow-triggers/DeleteWorkflowTrigger";

export function WorkflowTriggerActions({ item }: { item: WorkflowTrigger }) {
    let path: string = "";

    if (item.type === "webhook") {
        path = $path("/workflow-triggers/webhooks/:webhook", {
            webhook: item.id,
        });
    } else if (item.type === "schedule") {
        path = $path("/workflow-triggers/schedule/:trigger", {
            trigger: item.id,
        });
    } else if (item.type === "email") {
        path = $path("/workflow-triggers/email/:receiver", {
            receiver: item.id,
        });
    }

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
                    <Link to={path} as="div">
                        <DropdownMenuItem>Edit</DropdownMenuItem>
                    </Link>

                    <DeleteWorkflowTrigger id={item.id} type={item.type} />
                </DropdownMenuGroup>
            </DropdownMenuContent>
        </DropdownMenu>
    );
}
