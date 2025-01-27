import { ColumnDef, createColumnHelper } from "@tanstack/react-table";
import { useMemo } from "react";
import { MetaFunction, useNavigate } from "react-router";
import { $path } from "safe-routes";
import useSWR, { preload } from "swr";

import { WorkflowTrigger } from "~/lib/model/workflow-trigger";
import { Workflow } from "~/lib/model/workflows";
import { CronJobApiService } from "~/lib/service/api/cronjobApiService";
import { EmailReceiverApiService } from "~/lib/service/api/emailReceiverApiService";
import { WebhookApiService } from "~/lib/service/api/webhookApiService";
import { WorkflowService } from "~/lib/service/api/workflowService";

import { DataTable } from "~/components/composed/DataTable";
import { CreateWorkflowTrigger } from "~/components/workflow-triggers/CreateWorkflowTrigger";
import { WorkflowTriggerActions } from "~/components/workflow-triggers/WorkflowTriggerActions";
import { useWorkflowTriggers } from "~/hooks/workflow-triggers/useWorkflowTriggers";

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
		preload(EmailReceiverApiService.getEmailReceivers.key(), () =>
			EmailReceiverApiService.getEmailReceivers()
		),
	]);

	return null;
}

export default function WorkflowTriggersPage() {
	const navigate = useNavigate();
	const getWorkflows = useSWR(
		WorkflowService.getWorkflows.key(),
		() => WorkflowService.getWorkflows(),
		{ fallbackData: [] }
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

	const { workflowTriggers } = useWorkflowTriggers();

	const onNavigate = (row: WorkflowTrigger): void => {
		switch (row.type) {
			case "webhook":
				navigate(
					$path("/workflow-triggers/webhooks/:webhook", {
						webhook: row.id,
					})
				);
				break;
			case "email":
				navigate(
					$path("/workflow-triggers/email/:receiver", {
						receiver: row.id,
					})
				);
				break;
			case "schedule":
				navigate(
					$path("/workflow-triggers/schedule/:trigger", {
						trigger: row.id,
					})
				);
		}
	};
	return (
		<div className="flex h-full flex-col space-y-4 p-8">
			<div className="flex items-center justify-between">
				<h2>Workflow Triggers</h2>

				<CreateWorkflowTrigger />
			</div>

			<div className="flex flex-col gap-4">
				<DataTable
					onRowClick={onNavigate}
					columns={getColumns()}
					data={workflowTriggers}
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
			columnHelper.accessor((row) => workflowMap[row.workflow]?.name, {
				header: "Workflow",
				cell: ({ getValue }) =>
					getValue() ?? (
						<span className="text-muted-foreground">
							Workflow Not Available
						</span>
					),
			}),
			columnHelper.display({
				id: "actions",
				cell: ({ row }) => (
					<div className="flex items-center justify-end">
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
