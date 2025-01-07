import { PlusIcon } from "lucide-react";
import { $path } from "safe-routes";

import { Button } from "~/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogHeader,
    DialogTitle,
    DialogTrigger,
} from "~/components/ui/dialog";
import { Link } from "~/components/ui/link";
import {
    Tooltip,
    TooltipContent,
    TooltipTrigger,
} from "~/components/ui/tooltip";

export function CreateWorkflowTrigger() {
    return (
        <Dialog>
            <DialogTrigger>
                <Button>
                    <PlusIcon /> Create Trigger
                </Button>
            </DialogTrigger>
            <DialogContent>
                <DialogHeader>
                    <DialogTitle>Create Workflow Trigger</DialogTitle>
                </DialogHeader>
                <DialogDescription>
                    Select the type of workflow trigger you want to create.
                </DialogDescription>
                <div className="flex flex-col w-full space-y-4">
                    <Tooltip>
                        <TooltipTrigger asChild>
                            <Link
                                to={$path("/workflow-triggers/webhooks/create")}
                                as="button"
                                variant="outline"
                            >
                                Webhook
                            </Link>
                        </TooltipTrigger>
                        <TooltipContent>
                            Set up a workflow to send real-time events.
                        </TooltipContent>
                    </Tooltip>

                    <Tooltip>
                        <TooltipTrigger asChild>
                            <Link
                                to={$path("/workflow-triggers/schedule/create")}
                                as="button"
                                variant="outline"
                            >
                                Schedule
                            </Link>
                        </TooltipTrigger>
                        <TooltipContent>
                            Set up a workflow to run on an interval.
                        </TooltipContent>
                    </Tooltip>

                    <Tooltip>
                        <TooltipTrigger asChild>
                            <Link
                                to={$path("/workflow-triggers/email/create")}
                                as="button"
                                variant="outline"
                            >
                                Email
                            </Link>
                        </TooltipTrigger>
                    </Tooltip>
                </div>
            </DialogContent>
        </Dialog>
    );
}
