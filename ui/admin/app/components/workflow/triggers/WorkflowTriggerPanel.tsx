import { WebhookIcon } from "lucide-react";

import { WorkflowTriggerType } from "~/lib/model/workflow-trigger";

import { Tabs, TabsContent, TabsList, TabsTrigger } from "~/components/ui/tabs";
import { WorkflowEmailTab } from "~/components/workflow/triggers/WorkflowEmailTab";
import { WorkflowScheduleTab } from "~/components/workflow/triggers/WorkflowScheduleTab";
import { WorkflowWebhookTab } from "~/components/workflow/triggers/WorkflowWebhookTab";

export function WorkflowTriggerPanel({ workflowId }: { workflowId: string }) {
    return (
        <div className="p-4 m-4 flex flex-col gap-4">
            <Tabs defaultValue={WorkflowTriggerType.Schedule}>
                <div className="flex items-center justify-between">
                    <h4 className="flex items-center gap-2">
                        <WebhookIcon className="w-4 h-4" />
                        Triggers
                    </h4>

                    <TabsList>
                        <TabsTrigger value={WorkflowTriggerType.Schedule}>
                            Schedule
                        </TabsTrigger>

                        <TabsTrigger value={WorkflowTriggerType.Email}>
                            Email
                        </TabsTrigger>

                        <TabsTrigger value={WorkflowTriggerType.Webhook}>
                            Webhook
                        </TabsTrigger>
                    </TabsList>
                </div>

                <TabsContent value={WorkflowTriggerType.Schedule}>
                    <WorkflowScheduleTab workflowId={workflowId} />
                </TabsContent>

                <TabsContent value={WorkflowTriggerType.Email}>
                    <WorkflowEmailTab workflowId={workflowId} />
                </TabsContent>

                <TabsContent value={WorkflowTriggerType.Webhook}>
                    <WorkflowWebhookTab workflowId={workflowId} />
                </TabsContent>
            </Tabs>
        </div>
    );
}
