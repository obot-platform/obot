import useSWR from "swr";

import { Agent } from "~/lib/model/agents";
import { AgentService } from "~/lib/service/api/agentService";

import { SelectModule } from "~/components/composed/SelectModule";
import {
    Tooltip,
    TooltipContent,
    TooltipTrigger,
} from "~/components/ui/tooltip";

type AgentSelectModuleProps = {
    onChange: (agents: string[]) => void;
    selection: string[];
};

export function AgentSelectModule(props: AgentSelectModuleProps) {
    const { data: agents } = useSWR(
        AgentService.getAgents.key(),
        AgentService.getAgents
    );

    return (
        <SelectModule
            selection={props.selection}
            onChange={props.onChange}
            renderDropdownItem={(agent) => <AgentText agent={agent} />}
            renderListItem={(agent) => <AgentText agent={agent} />}
            getItemKey={(agent) => agent.id}
            buttonText="Add Agent"
            items={agents}
        />
    );
}

function AgentText({ agent }: { agent: Agent }) {
    const content = (
        <div className="flex items-center gap-2 overflow-hidden">
            <span className="min-w-fit">{agent.name}</span>
            {agent.description && (
                <>
                    <span>-</span>
                    <span className="text-muted-foreground truncate">
                        {agent.description}
                    </span>
                </>
            )}
        </div>
    );

    return (
        <Tooltip>
            <TooltipTrigger asChild>{content}</TooltipTrigger>
            <TooltipContent>{content}</TooltipContent>
        </Tooltip>
    );
}
