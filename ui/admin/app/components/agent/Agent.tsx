import { LibraryIcon, PlusIcon, WrenchIcon } from "lucide-react";
import { useCallback, useEffect, useState } from "react";

import { Agent as AgentType } from "~/lib/model/agents";
import { cn } from "~/lib/utils";

import { TypographyH4, TypographyP } from "~/components/Typography";
import { useAgent } from "~/components/agent/AgentContext";
import { AgentForm } from "~/components/agent/AgentForm";
import { AgentPublishStatus } from "~/components/agent/AgentPublishStatus";
import { PastThreads } from "~/components/agent/PastThreads";
import { ToolForm } from "~/components/agent/ToolForm";
import { AgentKnowledgePanel } from "~/components/knowledge";
import { Button } from "~/components/ui/button";
import { CardDescription } from "~/components/ui/card";
import { ScrollArea } from "~/components/ui/scroll-area";
import { useDebounce } from "~/hooks/useDebounce";

type AgentProps = {
    className?: string;
    onRefresh?: (threadId: string | null) => void;
};

export function Agent({ className, onRefresh }: AgentProps) {
    const { agent, updateAgent, isUpdating, lastUpdated, error } = useAgent();

    const [agentUpdates, setAgentUpdates] = useState(agent);

    useEffect(() => {
        setAgentUpdates((prev) => {
            if (agent.id === prev.id) {
                return {
                    ...prev,
                    aliasAssigned: agent.aliasAssigned ?? false,
                };
            }

            return agent;
        });
    }, [agent]);

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
                <AgentPublishStatus
                    agent={agentUpdates}
                    isUpdating={isUpdating}
                    onChange={partialSetAgent}
                />

                <div className="p-4 m-4 lg:mx-6 xl:mx-8">
                    <AgentForm
                        agent={agentUpdates}
                        onChange={debouncedSetAgentInfo}
                    />
                </div>

                <div className="p-4 m-4 space-y-4 lg:mx-6 xl:mx-8">
                    <TypographyH4 className="flex items-center gap-2 border-b pb-2">
                        <WrenchIcon className="w-5 h-5" />
                        Tools
                    </TypographyH4>

                    <CardDescription>
                        Add tools the allow the agent to perform useful actions
                        such as searching the web, reading files, or interacting
                        with other systems.
                    </CardDescription>

                    <ToolForm
                        agent={agentUpdates}
                        onChange={({ tools }) =>
                            debouncedSetAgentInfo(convertTools(tools))
                        }
                    />
                </div>

                <div className="p-4 m-4 space-y-4 lg:mx-6 xl:mx-8">
                    <TypographyH4 className="flex items-center gap-2 border-b pb-2">
                        <LibraryIcon className="w-6 h-6" />
                        Knowledge
                    </TypographyH4>
                    <CardDescription>
                        Provide knowledge to the agent in the form of files,
                        website, or external links in order to give it context
                        about various topics.
                    </CardDescription>
                    <AgentKnowledgePanel
                        agentId={agent.id}
                        agent={agent}
                        updateAgent={debouncedSetAgentInfo}
                    />
                </div>
            </ScrollArea>

            <footer className="flex justify-between items-center px-8 py-4 gap-4 shadow-inner">
                <div className="text-muted-foreground">
                    {error ? (
                        <TypographyP>Error saving agent</TypographyP>
                    ) : isUpdating ? (
                        <TypographyP>Saving...</TypographyP>
                    ) : lastUpdated ? (
                        <TypographyP>Saved</TypographyP>
                    ) : (
                        <div />
                    )}
                </div>

                <div className="flex gap-2">
                    <PastThreads
                        agentId={agent.id}
                        onThreadSelect={handleThreadSelect}
                    />

                    <Button
                        variant="outline"
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

function convertTools(
    tools: { tool: string; variant: "fixed" | "default" | "available" }[]
) {
    type ToolObj = Pick<
        AgentType,
        "tools" | "defaultThreadTools" | "availableThreadTools"
    >;

    return tools.reduce(
        (acc, { tool, variant }) => {
            if (variant === "fixed") acc.tools?.push(tool);
            else if (variant === "default") acc.defaultThreadTools?.push(tool);
            else if (variant === "available")
                acc.availableThreadTools?.push(tool);

            return acc;
        },
        {
            tools: [],
            defaultThreadTools: [],
            availableThreadTools: [],
        } as ToolObj
    );
}
