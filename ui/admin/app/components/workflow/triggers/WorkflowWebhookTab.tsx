import { WorkflowTriggerType } from "~/lib/model/workflow-trigger";

import { CardDescription } from "~/components/ui/card";
import { DeleteWorkflowTrigger } from "~/components/workflow-triggers/DeleteWorkflowTrigger";
import { WorkflowWebhookDialog } from "~/components/workflow/triggers/WorkflowWebhookDialog";
import { useWorkflowTriggers } from "~/hooks/workflow-triggers/useWorkflowTriggers";

export function WorkflowWebhookTab({ workflowId }: { workflowId: string }) {
	const { webhooks } = useWorkflowTriggers({ workflowId });

	return (
		<div className="flex flex-col gap-4">
			<CardDescription>
				Add webhooks to notify external services when your AI agent completes
				tasks or receives new information.
			</CardDescription>

			<div className="flex flex-col gap-2">
				{webhooks?.map((webhook) => (
					<div key={webhook.id} className="flex justify-between">
						<p>{webhook.name || webhook.id}</p>

						<div className="flex gap-2">
							<WorkflowWebhookDialog
								workflowId={workflowId}
								webhook={webhook}
							/>

							<DeleteWorkflowTrigger
								type={WorkflowTriggerType.Webhook}
								id={webhook.id}
							/>
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
