import { useNavigate } from "@remix-run/react";
import { ColumnDef, createColumnHelper } from "@tanstack/react-table";
import { EllipsisIcon, PlusIcon } from "lucide-react";
import { useMemo } from "react";
import { $path } from "remix-routes";
import useSWR, { preload } from "swr";

import { Webhook } from "~/lib/model/webhooks";
import { Workflow } from "~/lib/model/workflows";
import { WebhookApiService } from "~/lib/service/api/webhookApiService";
import { WorkflowService } from "~/lib/service/api/workflowService";

import { TypographyH2 } from "~/components/Typography";
import { DataTable } from "~/components/composed/DataTable";
import { Button } from "~/components/ui/button";
import { Link } from "~/components/ui/link";
import {
    Popover,
    PopoverContent,
    PopoverTrigger,
} from "~/components/ui/popover";
import { DeleteWebhook } from "~/components/webhooks/DeleteWebhook";

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
                    buttonVariant="outline"
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
                cell: ({ row }) => {
                    return (
                        <div className="flex items-center justify-end">
                            <Popover>
                                <PopoverTrigger asChild>
                                    <Button
                                        variant="ghost"
                                        size="icon"
                                        onClick={(e) => e.stopPropagation()}
                                    >
                                        <EllipsisIcon />
                                    </Button>
                                </PopoverTrigger>

                                <PopoverContent
                                    className="w-48 p-2 flex flex-col gap-1"
                                    side="top"
                                    align="end"
                                >
                                    <Link
                                        to={$path("/webhooks/:webhook", {
                                            webhook: row.id,
                                        })}
                                        as="button"
                                        buttonSize="sm"
                                    >
                                        Edit
                                    </Link>

                                    <DeleteWebhook id={row.original.id}>
                                        <Button
                                            variant="destructive"
                                            size="sm"
                                            className="w-full"
                                            onClick={(e) => e.stopPropagation()}
                                        >
                                            Delete
                                        </Button>
                                    </DeleteWebhook>
                                </PopoverContent>
                            </Popover>
                        </div>
                    );
                },
            }),
        ];
    }
}

const columnHelper = createColumnHelper<Webhook>();
