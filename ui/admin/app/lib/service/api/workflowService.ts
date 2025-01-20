import {
	CreateWorkflow,
	UpdateWorkflow,
	Workflow,
} from "~/lib/model/workflows";
import { ApiRoutes, revalidateWhere } from "~/lib/routers/apiRoutes";
import { ResponseHeaders, request } from "~/lib/service/api/primitives";

async function getWorkflows() {
	const res = await request<{ items: Workflow[] }>({
		url: ApiRoutes.workflows.base().url,
		errorMessage: "Failed to fetch workflows",
	});

	return res.data.items ?? ([] as Workflow[]);
}
getWorkflows.key = () => ({ url: ApiRoutes.workflows.base().path }) as const;

const getWorkflowById = async (workflowId: string) => {
	const res = await request<Workflow>({
		url: ApiRoutes.workflows.getById(workflowId).url,
		errorMessage: "Failed to fetch workflow",
	});

	return res.data;
};
getWorkflowById.key = (workflowId?: Nullish<string>) => {
	if (!workflowId) return null;

	return { url: ApiRoutes.workflows.getById(workflowId).path, workflowId };
};

async function createWorkflow(workflow: CreateWorkflow) {
	const res = await request<Workflow>({
		url: ApiRoutes.workflows.base().url,
		method: "POST",
		data: workflow,
		errorMessage: "Failed to create workflow",
	});

	return res.data;
}

async function updateWorkflow({
	id,
	workflow,
}: {
	id: string;
	workflow: UpdateWorkflow;
}) {
	const res = await request<Workflow>({
		url: ApiRoutes.workflows.getById(id).url,
		method: "PUT",
		data: workflow,
		errorMessage: "Failed to update workflow",
	});

	return res.data;
}

async function deleteWorkflow(id: string, deleteTriggers?: boolean) {
	await request({
		url: ApiRoutes.workflows.getById(id).url,
		method: "DELETE",
		params: deleteTriggers ? { "delete-triggers": true } : undefined,
		errorMessage: "Failed to delete workflow",
	});
}

const revalidateWorkflows = () =>
	revalidateWhere((url) => url.includes(ApiRoutes.workflows.base().path));

async function authenticateWorkflow(workflowId: string) {
	const response = await request<ReadableStream>({
		url: ApiRoutes.workflows.authenticate(workflowId).url,
		method: "POST",
		headers: { Accept: "text/event-stream" },
		responseType: "stream",
		errorMessage: "Failed to invoke agenticate workflow",
	});

	const reader = response.data
		?.pipeThrough(new TextDecoderStream())
		.getReader();

	const threadId = response.headers[ResponseHeaders.ThreadId] as string;

	return { reader, threadId };
}

export const WorkflowService = {
	getWorkflows,
	getWorkflowById,
	createWorkflow,
	updateWorkflow,
	deleteWorkflow,
	revalidateWorkflows,
	authenticateWorkflow,
};
