import { LibraryIcon, PlusIcon, WrenchIcon } from "lucide-react";
import { useCallback, useState } from "react";

import { Agent as AgentType } from "~/lib/model/agents";
import { cn } from "~/lib/utils";

import { TypographyH4, TypographyP } from "~/components/Typography";
import { useAgent } from "~/components/agent/AgentContext";
import { AgentForm } from "~/components/agent/AgentForm";
import { PastThreads } from "~/components/agent/PastThreads";
import { Publish } from "~/components/agent/Publish";
import { ToolForm } from "~/components/agent/ToolForm";
import { Unpublish } from "~/components/agent/Unpublish";
import { CopyText } from "~/components/composed/CopyText";
import { AgentKnowledgePanel } from "~/components/knowledge";
import { Button } from "~/components/ui/button";
import { Card } from "~/components/ui/card";
import { ScrollArea } from "~/components/ui/scroll-area";
import { useDebounce } from "~/hooks/useDebounce";

type AgentProps = {
    agent: AgentType;
    className?: string;
    onRefresh?: (threadId: string | null) => void;
};

export function Agent({ className, onRefresh }: AgentProps) {
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
                <div className="flex w-full justify-between px-8 pt-4 items-center gap-4">
                    {agentUpdates.refName ? (
                        <CopyText
                            className="h-8 text-muted-foreground text-sm bg-background flex-row-reverse"
                            holdStatusDelay={10000}
                            text={`${window.location.protocol}//${window.location.host}/${agentUpdates.refName}`}
                        />
                    ) : (
                        <div />
                    )}

                    {agentUpdates.refName ? (
                        <Unpublish onChange={debouncedSetAgentInfo} />
                    ) : (
                        <Publish
                            agent={agentUpdates}
                            onChange={debouncedSetAgentInfo}
                        />
                    )}
                </div>
                <Card className="p-4 m-4 lg:mx-6 xl:mx-8">
                    <AgentForm
                        agent={agentUpdates}
                        onChange={debouncedSetAgentInfo}
                    />
                </Card>

                <Card className="p-4 m-4 space-y-4 lg:mx-6 xl:mx-8">
                    <TypographyH4 className="flex items-center gap-2 border-b pb-2">
                        <WrenchIcon className="w-5 h-5" />
                        Tools
                    </TypographyH4>

                    <TypographyP className="text-muted-foreground flex items-center gap-2">
                        Add tools the allow the agent to perform useful actions
                        such as searching the web, reading files, or interacting
                        with other systems.
                    </TypographyP>

                    <ToolForm
                        agent={agentUpdates}
                        onChange={({ tools }) =>
                            debouncedSetAgentInfo(convertTools(tools))
                        }
                    />
                </Card>

                <Card className="p-4 m-4 space-y-4 lg:mx-6 xl:mx-8">
                    <TypographyH4 className="flex items-center gap-2 border-b pb-2">
                        <LibraryIcon className="w-6 h-6" />
                        Knowledge
                    </TypographyH4>
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
                </Card>
            </ScrollArea>

            <footer className="flex justify-between items-center px-8 py-4 gap-4 text-muted-foreground">
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
