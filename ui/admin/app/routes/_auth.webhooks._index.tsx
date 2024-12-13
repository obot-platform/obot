import { ColumnDef, createColumnHelper } from "@tanstack/react-table";
import { PlusIcon } from "lucide-react";
import { useMemo } from "react";
import { useNavigate } from "react-router";
import { $path } from "safe-routes";
import useSWR, { preload } from "swr";

import { Webhook } from "~/lib/model/webhooks";
import { Workflow } from "~/lib/model/workflows";
import { WebhookApiService } from "~/lib/service/api/webhookApiService";
import { WorkflowService } from "~/lib/service/api/workflowService";

import { TypographyH2 } from "~/components/Typography";
import { DataTable } from "~/components/composed/DataTable";
import { Link } from "~/components/ui/link";
import { WebhookActions } from "~/components/webhooks/WebhookActions";

export async function clientLoader() {
    await Promise.all([
        preload(WebhookApiService.getWebhooks.key(), () =>
            WebhookApiService.getWebhooks()
        ),
        preload(WorkflowService.getWorkflows.key(), () =>
            WorkflowService.getWorkflows()
        ),
    ]);

    return null;
}

export default function WebhooksPage() {
    const { data: webhooks } = useSWR(WebhookApiService.getWebhooks.key(), () =>
        WebhookApiService.getWebhooks()
    );

    const navigate = useNavigate();

    const getWorkflows = useSWR(WorkflowService.getWorkflows.key(), () =>
        WorkflowService.getWorkflows()
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

    return (
        <div className="h-full flex flex-col p-8 space-y-4">
            <div className="flex items-center justify-between">
                <TypographyH2>Webhooks</TypographyH2>

                <Link
                    to={$path("/webhooks/create")}
                    as="button"
                    variant="outline"
                >
                    <PlusIcon /> Create Webhook
                </Link>
            </div>

            <div className="flex flex-col gap-4">
                <DataTable
                    onRowClick={(row) =>
                        navigate(
                            $path("/webhooks/:webhook", { webhook: row.id })
                        )
                    }
                    columns={getColumns()}
                    data={webhooks ?? []}
                />
            </div>
        </div>
    );

    function getColumns(): ColumnDef<Webhook, string>[] {
        return [
            columnHelper.accessor("name", { header: "Name" }),
            columnHelper.accessor(
                (row) => workflowMap[row.workflow]?.name ?? row.workflow,
                { header: "Workflow" }
            ),
            columnHelper.display({
                id: "actions",
                cell: ({ row }) => (
                    <div className="flex items-center justify-end ">
                        <WebhookActions webhook={row.original} />
                    </div>
                ),
            }),
        ];
    }
}

const columnHelper = createColumnHelper<Webhook>();
