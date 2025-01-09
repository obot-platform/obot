import useSWR from "swr";

import {
    WorkflowTriggerType,
    collateWorkflowTriggers,
} from "~/lib/model/workflow-trigger";
import { CronJobApiService } from "~/lib/service/api/cronjobApiService";
import { EmailReceiverApiService } from "~/lib/service/api/emailReceiverApiService";
import { WebhookApiService } from "~/lib/service/api/webhookApiService";

type UseWorkflowTriggersProps = {
    type?: WorkflowTriggerType | WorkflowTriggerType[];
    workflowId?: string;
};

const AllTypes = Object.values(WorkflowTriggerType);

export function useWorkflowTriggers(props?: UseWorkflowTriggersProps) {
    const { type = AllTypes, workflowId } = props ?? {};

    const types = new Set(Array.isArray(type) ? type : [type]);

    const { data: emailReceivers } = useSWR(
        types.has("email") &&
            EmailReceiverApiService.getEmailReceivers.key({ workflowId }),
        ({ filters }) => EmailReceiverApiService.getEmailReceivers(filters),
        { fallbackData: [] }
    );

    const { data: cronjobs } = useSWR(
        types.has("schedule") &&
            CronJobApiService.getCronJobs.key({ workflowId }),
        ({ filters }) => CronJobApiService.getCronJobs(filters),
        { fallbackData: [] }
    );

    const { data: webhooks } = useSWR(
        types.has("webhook") && WebhookApiService.getWebhooks.key(),
        () => WebhookApiService.getWebhooks(),
        { fallbackData: [] }
    );

    return {
        workflowTriggers: getFilteredTriggers(),
        emailReceivers,
        cronjobs,
        webhooks,
    };

    function getFilteredTriggers() {
        const workflowTriggers = collateWorkflowTriggers(
            [
                types.has("email") && emailReceivers,
                types.has("schedule") && cronjobs,
                types.has("webhook") && webhooks,
            ]
                .filter((x) => !!x)
                .flat()
        );

        if (workflowId) {
            return workflowTriggers.filter((x) => x.workflow === workflowId);
        }

        return workflowTriggers;
    }
}
