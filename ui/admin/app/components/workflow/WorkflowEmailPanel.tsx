import { MailIcon } from "lucide-react";
import useSWR from "swr";

import { EmailReceiverApiService } from "~/lib/service/api/emailReceiverApiService";

import { CopyText } from "~/components/composed/CopyText";
import { CardDescription } from "~/components/ui/card";
import { DeleteWorkflowEmailReceiver } from "~/components/workflow/DeleteWorkflowEmailReceiver";
import { WorkflowEmailDialog } from "~/components/workflow/WorkflowEmailDialog";

export function WorkflowEmailPanel({ workflowId }: { workflowId: string }) {
    const { data: receivers } = useSWR(
        EmailReceiverApiService.getEmailReceivers.key(),
        EmailReceiverApiService.getEmailReceivers
    );

    const workflowReceivers = receivers?.filter(
        (receiver) => receiver.workflow === workflowId
    );

    return (
        <div className="p-4 m-4 flex flex-col gap-4">
            <h4 className="flex items-center gap-2">
                <MailIcon className="w-4 h-4" />
                Email Triggers
            </h4>

            <CardDescription>
                Add Email Triggers to run the workflow when an email is
                received.
            </CardDescription>

            <div className="flex flex-col gap-2">
                {workflowReceivers?.map((receiver) => (
                    <div
                        key={receiver.id}
                        className="flex justify-between items-center"
                    >
                        <p>{receiver.name || receiver.id}</p>

                        <div className="flex gap-2">
                            <CopyText
                                text={receiver.emailAddress}
                                className="bg-transparent text-muted-foreground text-sm"
                                classNames={{
                                    text: "p-0",
                                }}
                                hideIcon
                            />

                            <WorkflowEmailDialog
                                workflowId={workflowId}
                                emailReceiver={receiver}
                            />

                            <DeleteWorkflowEmailReceiver
                                emailReceiverId={receiver.id}
                            />
                        </div>
                    </div>
                ))}
            </div>

            <div className="flex justify-end">
                <WorkflowEmailDialog workflowId={workflowId} />
            </div>
        </div>
    );
}
