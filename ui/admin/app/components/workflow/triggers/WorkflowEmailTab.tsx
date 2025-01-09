import { CardDescription } from "~/components/ui/card";
import { EmailTriggerEntry } from "~/components/workflow/triggers/EmailReceiverEntry";
import { WorkflowEmailDialog } from "~/components/workflow/triggers/WorkflowEmailDialog";
import { useWorkflowTriggers } from "~/hooks/workflow-triggers/useWorkflowTriggers";

export function WorkflowEmailTab({ workflowId }: { workflowId: string }) {
    const { emailReceivers } = useWorkflowTriggers({ workflowId });

    return (
        <div className="flex flex-col gap-2">
            <CardDescription>
                Add Email Triggers to run the workflow when an email
            </CardDescription>

            {emailReceivers.map((emailReceiver) => (
                <EmailTriggerEntry
                    key={emailReceiver.id}
                    receiver={emailReceiver}
                    workflowId={workflowId}
                />
            ))}

            <div className="self-end">
                <WorkflowEmailDialog workflowId={workflowId} />
            </div>
        </div>
    );
}
