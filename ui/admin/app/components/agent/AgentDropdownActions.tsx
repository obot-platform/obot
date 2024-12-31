import { EllipsisVerticalIcon } from "lucide-react";
import { useNavigate } from "react-router";
import { $path } from "safe-routes";
import { toast } from "sonner";
import { mutate } from "swr";

import { Agent } from "~/lib/model/agents";
import { AgentService } from "~/lib/service/api/agentService";

import { ConfirmationDialog } from "~/components/composed/ConfirmationDialog";
import { Button } from "~/components/ui/button";
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuTrigger,
} from "~/components/ui/dropdown-menu";
import { useConfirmationDialog } from "~/hooks/component-helpers/useConfirmationDropdown";
import { useAsync } from "~/hooks/useAsync";

export function AgentDropdownActions({ agent }: { agent: Agent }) {
    const navigate = useNavigate();

    const deleteAgent = useAsync(AgentService.deleteAgent, {
        onSuccess: () => {
            mutate(AgentService.getAgents.key());
            toast.success("Agent deleted");
            navigate($path("/agents"));
        },
        onError: (error) => {
            if (error instanceof Error) return toast.error(error.message);

            toast.error("Something went wrong");
        },
    });

    const { dialogProps, interceptAsync } = useConfirmationDialog();

    const handleDelete = () =>
        interceptAsync(() => deleteAgent.executeAsync(agent.id));

    return (
        <>
            <DropdownMenu modal>
                <DropdownMenuTrigger>
                    <Button size="icon" variant="ghost">
                        <EllipsisVerticalIcon className="w-4 h-4" />
                    </Button>
                </DropdownMenuTrigger>

                <DropdownMenuContent align="end">
                    <DropdownMenuItem
                        variant="destructive"
                        onClick={handleDelete}
                    >
                        Delete Agent
                    </DropdownMenuItem>
                </DropdownMenuContent>
            </DropdownMenu>

            <ConfirmationDialog
                {...dialogProps}
                title="Are you sure you want to delete this agent?"
                description="This action cannot be undone."
                confirmProps={{
                    variant: "destructive",
                    loading: deleteAgent.isLoading,
                    disabled: deleteAgent.isLoading,
                }}
            />
        </>
    );
}
