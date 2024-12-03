import { useNavigate } from "@remix-run/react";
import { PlusIcon } from "lucide-react";
import { $path } from "remix-routes";
import { toast } from "sonner";
import { mutate } from "swr";

import { WorkflowService } from "~/lib/service/api/workflowService";
import { generateRandomName } from "~/lib/service/nameGenerator";

import { useModelProviders } from "~/components/model-providers/ModelProviderContext";
import { Button } from "~/components/ui/button";
import { useAsync } from "~/hooks/useAsync";

export function CreateWorkflow() {
    const navigate = useNavigate();
    const { configured: modelProviderConfigured } = useModelProviders();

    const createWorkflow = useAsync(WorkflowService.createWorkflow, {
        onSuccess: (res) => {
            mutate(WorkflowService.getWorkflows.key());
            toast.success("Workflow created");
            navigate($path("/workflows/:workflow", { workflow: res.id }));
        },
        onError: () => toast.error("Failed to create workflow"),
    });

    return (
        <Button
            variant="outline"
            disabled={!modelProviderConfigured}
            startContent={<PlusIcon />}
            onClick={() =>
                createWorkflow.execute({ name: generateRandomName() })
            }
            loading={createWorkflow.isLoading}
        >
            Create Workflow
        </Button>
    );
}
