import { toast } from "sonner";

import { Workflow } from "~/lib/model/workflows";
import { WorkflowService } from "~/lib/service/api/workflowService";

import { EnvForm } from "~/components/agent/shared/AgentEnvironmentVariableForm";
import { Button } from "~/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogHeader,
    DialogTitle,
    DialogTrigger,
} from "~/components/ui/dialog";
import { useAsync } from "~/hooks/useAsync";

type WorkflowEnvFormProps = {
    workflow: Workflow;
};

export function WorkflowEnvSection({ workflow }: WorkflowEnvFormProps) {
    // use useAsync because we don't want to cache env variables
    const revealEnv = useAsync(WorkflowService.getWorkflowEnv);

    const onOpenChange = (open: boolean) => {
        if (open) {
            revealEnv.execute(workflow.id);
        } else {
            updateEnv.clear();
        }
    };

    const updateEnv = useAsync(WorkflowService.updateWorkflowEnv, {
        onSuccess: () => {
            toast.success("Environment variables updated");
            revealEnv.clear();
        },
    });

    const open = !!revealEnv.data;

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogTrigger asChild>
                <Button loading={revealEnv.isLoading}>
                    Environment Variables
                </Button>
            </DialogTrigger>

            <DialogContent>
                <DialogHeader>
                    <DialogTitle>Environment Variables</DialogTitle>
                </DialogHeader>

                <DialogDescription>
                    Environment variables are used to store values that can be
                    used in your workflow.
                </DialogDescription>

                {revealEnv.data && (
                    <EnvForm
                        defaultValues={revealEnv.data}
                        isLoading={updateEnv.isLoading}
                        onSubmit={(values) =>
                            updateEnv.execute(workflow.id, values)
                        }
                    />
                )}
            </DialogContent>
        </Dialog>
    );
}
