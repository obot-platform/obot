import {
    CreateWorkflow,
    UpdateWorkflow,
    Workflow,
} from "~/lib/model/workflows";
import { ApiRoutes, revalidateWhere } from "~/lib/routers/apiRoutes";
import { request } from "~/lib/service/api/primitives";

async function getWorkflows() {
    const res = await request<{ items: Workflow[] }>({
        url: ApiRoutes.workflows.base().url,
    });

    return res.data.items ?? ([] as Workflow[]);
}
getWorkflows.key = () => ({ url: ApiRoutes.workflows.base().path }) as const;

const getWorkflowById = async (workflowId: string) => {
    const res = await request<Workflow>({
        url: ApiRoutes.workflows.getById(workflowId).url,
    });

    return res.data;
};
getWorkflowById.key = (workflowId?: Nullish<string>) => {
    if (!workflowId) return null;

    return { url: ApiRoutes.workflows.getById(workflowId).path, workflowId };
};

async function createWorkflow({ workflow }: { workflow: CreateWorkflow }) {
    const res = await request<Workflow>({
        url: ApiRoutes.workflows.base().url,
        method: "POST",
        data: workflow,
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
    });

    return res.data;
}

async function deleteWorkflow(id: string) {
    await request({
        url: ApiRoutes.workflows.getById(id).url,
        method: "DELETE",
    });
}

const revalidateWorkflows = () =>
    revalidateWhere((url) => url.includes(ApiRoutes.workflows.base().path));

export const WorkflowService = {
    getWorkflows,
    getWorkflowById,
    createWorkflow,
    updateWorkflow,
    deleteWorkflow,
    revalidateWorkflows,
};
