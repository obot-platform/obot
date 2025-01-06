import { ColumnDef, createColumnHelper } from "@tanstack/react-table";
import { useMemo } from "react";
import { MetaFunction, useNavigate } from "react-router";
import { $path } from "safe-routes";
import useSWR, { preload } from "swr";

import { WorkflowTrigger } from "~/lib/model/workflow-trigger";
import { Workflow } from "~/lib/model/workflows";
import { CronJobApiService } from "~/lib/service/api/cronjobApiService";
import { WebhookApiService } from "~/lib/service/api/webhookApiService";
import { WorkflowService } from "~/lib/service/api/workflowService";

import { TypographyH2 } from "~/components/Typography";
import { DataTable } from "~/components/composed/DataTable";
import { CreateWorkflowTrigger } from "~/components/workflow-triggers/CreateWorkflowTrigger";
import { WorkflowTriggerActions } from "~/components/workflow-triggers/WorkflowTriggerActions";

export async function clientLoader() {
    await Promise.all([
        preload(WebhookApiService.getWebhooks.key(), () =>
            WebhookApiService.getWebhooks()
        ),
        preload(WorkflowService.getWorkflows.key(), () =>
            WorkflowService.getWorkflows()
        ),
        preload(CronJobApiService.getCronJobs.key(), () =>
            CronJobApiService.getCronJobs()
        ),
    ]);

    return null;
}

export default function WorkflowTriggersPage() {
    const { data: webhooks } = useSWR(WebhookApiService.getWebhooks.key(), () =>
        WebhookApiService.getWebhooks()
    );

    const navigate = useNavigate();

    const getWorkflows = useSWR(WorkflowService.getWorkflows.key(), () =>
        WorkflowService.getWorkflows()
    );

    const { data: cronjobs } = useSWR(CronJobApiService.getCronJobs.key(), () =>
        CronJobApiService.getCronJobs()
    );

    const workflows = getWorkflows.data;

    const workflowMap = useMemo(() => {
        if (!workflows) return {};
        return workflows.reduce(
            (acc, workflow) => {
                acc[workflow.id] = workflow;
                return acc;
            },
            {} as Record<string, Workflow>
        );
    }, [workflows]);

    const tableData: WorkflowTrigger[] = [
        ...(webhooks ?? []),
        ...(cronjobs ?? []),
    ].map((item) =>
        "schedule" in item
            ? {
                  id: item.id,
                  type: "schedule",
                  name: item.id,
                  workflow: item.workflow,
              }
            : {
                  id: item.id,
                  type: "webhook",
                  name: item.name || item.id,
                  workflow: item.workflow,
              }
    );

    return (
        <div className="h-full flex flex-col p-8 space-y-4">
            <div className="flex items-center justify-between">
                <TypographyH2>Workflow Triggers</TypographyH2>

                <CreateWorkflowTrigger />
            </div>

            <div className="flex flex-col gap-4">
                <DataTable
                    onRowClick={(row) => {
                        if (row.type === "webhook") {
                            navigate(
                                $path("/workflow-triggers/webhooks/:webhook", {
                                    webhook: row.id,
                                })
                            );
                        } else {
                            navigate(
                                $path("/workflow-triggers/schedule/:trigger", {
                                    trigger: row.id,
                                })
                            );
                        }
                    }}
                    columns={getColumns()}
                    data={tableData}
                />
            </div>
        </div>
    );

    function getColumns(): ColumnDef<WorkflowTrigger, string>[] {
        return [
            columnHelper.accessor("name", { header: "Name" }),
            columnHelper.accessor((row) => row.type as string, {
                header: "Type",
            }),
            columnHelper.accessor(
                (row) => workflowMap[row.workflow]?.name ?? row.workflow,
                { header: "Workflow" }
            ),
            columnHelper.display({
                id: "actions",
                cell: ({ row }) => (
                    <div className="flex items-center justify-end ">
                        <WorkflowTriggerActions item={row.original} />
                    </div>
                ),
            }),
        ];
    }
}

const columnHelper = createColumnHelper<WorkflowTrigger>();

export const meta: MetaFunction = () => {
    return [{ title: `Obot • Workflow Triggers` }];
};
