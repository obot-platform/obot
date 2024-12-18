import { toast } from "sonner";

import { Agent } from "~/lib/model/agents";
import { Workflow } from "~/lib/model/workflows";
import { EnvironmentApiService } from "~/lib/service/api/EnvironmentApiService";

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

type AgentEnvFormProps = {
    entity: Agent | Workflow;
    entityType: "agent" | "workflow";
};

export function AgentEnvSection({ entity, entityType }: AgentEnvFormProps) {
    // use useAsync because we don't want to cache env variables
    const revealEnv = useAsync(EnvironmentApiService.getEnvVariables);

    const onOpenChange = (open: boolean) => {
        if (open) {
            revealEnv.execute(entity.id);
        } else {
            revealEnv.clear();
        }
    };

    const updateEnv = useAsync(EnvironmentApiService.updateEnvVariables, {
        onSuccess: () => {
            toast.success("Environment variables updated");
            revealEnv.clear();
        },
    });

    const open = !!revealEnv.data;

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogTrigger asChild>
                <Button loading={revealEnv.isLoading} className="w-full">
                    Environment Variables
                </Button>
            </DialogTrigger>

            <DialogContent className="max-w-3xl">
                <DialogHeader>
                    <DialogTitle>Environment Variables</DialogTitle>
                </DialogHeader>

                <DialogDescription>
                    Environment variables are used to store values that can be
                    used in your {entityType}.
                </DialogDescription>

                {revealEnv.data && (
                    <EnvForm
                        defaultValues={revealEnv.data}
                        isLoading={updateEnv.isLoading}
                        onSubmit={(values) =>
                            updateEnv.execute(entity.id, values)
                        }
                    />
                )}
            </DialogContent>
        </Dialog>
    );
}
