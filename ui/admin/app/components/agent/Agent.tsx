import { LibraryIcon, PlusIcon, WrenchIcon } from "lucide-react";
import { useCallback, useState } from "react";

import { Agent as AgentType } from "~/lib/model/agents";
import { cn } from "~/lib/utils";

import { TypographyP } from "~/components/Typography";
import { AgentProvider, useAgent } from "~/components/agent/AgentContext";
import { AgentForm } from "~/components/agent/AgentForm";
import { PastThreads } from "~/components/agent/PastThreads";
import { ToolForm } from "~/components/agent/ToolForm";
import { AgentKnowledgePanel } from "~/components/knowledge";
import { Button } from "~/components/ui/button";
import { ScrollArea } from "~/components/ui/scroll-area";
import { useDebounce } from "~/hooks/useDebounce";

type AgentProps = {
    agent: AgentType;
    className?: string;
    onRefresh?: (threadId: string | null) => void;
};

export function Agent(props: AgentProps) {
    return (
        <AgentProvider agent={props.agent}>
            <AgentContent {...props} />
        </AgentProvider>
    );
}

function AgentContent({ className, onRefresh }: AgentProps) {
    const { agent, updateAgent, isUpdating, lastUpdated } = useAgent();

    const [agentUpdates, setAgentUpdates] = useState(agent);

    const partialSetAgent = useCallback(
        (changes: Partial<typeof agent>) => {
            const updatedAgent = { ...agent, ...agentUpdates, ...changes };

            updateAgent(updatedAgent);

            setAgentUpdates(updatedAgent);
        },
        [agentUpdates, updateAgent, agent]
    );

    const debouncedSetAgentInfo = useDebounce(partialSetAgent, 1000);

    const handleThreadSelect = useCallback(
        (threadId: string) => {
            onRefresh?.(threadId);
        },
        [onRefresh]
    );

    return (
        <div className="h-full flex flex-col">
            <ScrollArea className={cn("h-full", className)}>
                <div className="p-4">
                    <AgentForm
                        agent={agentUpdates}
                        onChange={debouncedSetAgentInfo}
                    />
                </div>

                <div className="p-4 flex-auto space-y-4">
                    <span className="flex items-center gap-2 text-xl">
                        <WrenchIcon className="w-5 h-5" />
                        Tools
                    </span>
                    <TypographyP className="text-muted-foreground flex items-center gap-2">
                        Add tools the allow the agent to perform useful actions
                        such as searching the web, reading files, or interacting
                        with other systems.
                    </TypographyP>
                    <ToolForm
                        agent={agentUpdates}
                        onChange={debouncedSetAgentInfo}
                    />
                </div>

                <div className="p-4 flex-auto space-y-4">
                    <span className="flex items-center gap-2 text-xl">
                        <LibraryIcon className="w-6 h-6" />
                        Knowledge
                    </span>
                    <TypographyP className="text-muted-foreground flex items-center gap-2">
                        Provide knowledge to the agent in the form of files,
                        website, or external links in order to give it context
                        about various topics.
                    </TypographyP>
                    <AgentKnowledgePanel
                        agentId={agent.id}
                        agent={agent}
                        updateAgent={debouncedSetAgentInfo}
                    />
                </div>
            </ScrollArea>

            <footer className="flex justify-between items-center p-4 gap-4 text-muted-foreground">
                {isUpdating ? (
                    <TypographyP>Saving...</TypographyP>
                ) : lastUpdated ? (
                    <TypographyP>Saved</TypographyP>
                ) : (
                    <div />
                )}

                <div className="flex gap-2">
                    <PastThreads
                        agentId={agent.id}
                        onThreadSelect={handleThreadSelect}
                    />
                    <Button
                        variant="secondary"
                        className="flex gap-2"
                        onClick={() => {
                            onRefresh?.(null);
                        }}
                    >
                        <PlusIcon className="w-4 h-4" />
                        New Thread
                    </Button>
                </div>
            </footer>
        </div>
    );
}
