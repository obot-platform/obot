import { WebhookIcon } from "lucide-react";
import useSWR from "swr";

import { WebhookApiService } from "~/lib/service/api/webhookApiService";

import { TypographyH4, TypographyP } from "~/components/Typography";
import { CardDescription } from "~/components/ui/card";
import { DeleteWorkflowWebhook } from "~/components/workflow/DeleteWorkflowWebhook";
import { WorkflowWebhookDialog } from "~/components/workflow/WorkflowWebhookDialog";

export function WorkflowWebhookPanel({ workflowId }: { workflowId: string }) {
    const { data: webhooks } = useSWR(
        WebhookApiService.getWebhooks.key(),
        WebhookApiService.getWebhooks
    );

    const workflowWebhooks = webhooks?.filter(
        (webhook) => webhook.workflow === workflowId
    );

    return (
        <div className="p-4 m-4 flex flex-col gap-4">
            <TypographyH4 className="flex items-center gap-2">
                <WebhookIcon className="w-4 h-4" />
                Webhooks
            </TypographyH4>

            <CardDescription>
                Add webhooks to notify external services when your AI agent
                completes tasks or receives new information.
            </CardDescription>

            <div className="flex flex-col gap-2">
                {workflowWebhooks?.map((webhook) => (
                    <div key={webhook.id} className="flex justify-between">
                        <TypographyP>{webhook.name || webhook.id}</TypographyP>

                        <div className="flex gap-2">
                            <WorkflowWebhookDialog
                                workflowId={workflowId}
                                webhook={webhook}
                            />
                            <DeleteWorkflowWebhook webhookId={webhook.id} />
                        </div>
                    </div>
                ))}
            </div>

            <div className="flex justify-end">
                <WorkflowWebhookDialog workflowId={workflowId} />
            </div>
        </div>
    );
}
