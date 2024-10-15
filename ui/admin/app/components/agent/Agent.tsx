import { LibraryIcon, RotateCcw, WrenchIcon } from "lucide-react";
import { useCallback, useState } from "react";

import { Agent as AgentType } from "~/lib/model/agents";
import { cn } from "~/lib/utils";

import { TypographyP } from "~/components/Typography";
import { AgentProvider, useAgent } from "~/components/agent/AgentContext";
import { AgentForm } from "~/components/agent/AgentForm";
import { AgentKnowledgePanel } from "~/components/knowledge";
import {
    Accordion,
    AccordionContent,
    AccordionItem,
    AccordionTrigger,
} from "~/components/ui/accordion";
import { Button } from "~/components/ui/button";
import { ScrollArea } from "~/components/ui/scroll-area";
import { useDebounce } from "~/hooks/useDebounce";

import { ToolForm } from "./ToolForm";

type AgentProps = {
    agent: AgentType;
    className?: string;
    onRefresh?: () => void;
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

    return (
        <div className="h-full flex flex-col">
            <ScrollArea className={cn("h-full", className)}>
                <div className="p-4">
                    <AgentForm
                        agent={agentUpdates}
                        onChange={debouncedSetAgentInfo}
                    />
                </div>

                <Accordion type="multiple" className="p-4 flex-auto">
                    <AccordionItem value="tools-form">
                        <AccordionTrigger>
                            <span className="flex items-center gap-2 justify-center">
                                <WrenchIcon className="w-4 h-4" />
                                Tools
                            </span>
                        </AccordionTrigger>
                        <AccordionContent className="p-2">
                            <ToolForm
                                agent={agentUpdates}
                                onChange={debouncedSetAgentInfo}
                            />
                        </AccordionContent>
                    </AccordionItem>

                    <AccordionItem value="knowledge-form">
                        <AccordionTrigger>
                            <span className="flex items-center gap-2 justify-center">
                                <LibraryIcon className="w-4 h-4" />
                                Dropping Knowledge
                            </span>
                        </AccordionTrigger>

                        <AccordionContent>
                            <AgentKnowledgePanel agentId={agent.id} />
                        </AccordionContent>
                    </AccordionItem>
                </Accordion>
            </ScrollArea>

            <footer className="flex justify-between items-center p-4 gap-4 text-muted-foreground">
                {isUpdating ? (
                    <TypographyP>Saving...</TypographyP>
                ) : lastUpdated ? (
                    <TypographyP>Saved</TypographyP>
                ) : (
                    <div />
                )}

                <Button
                    variant="secondary"
                    className="flex gap-2"
                    onClick={onRefresh}
                >
                    <RotateCcw className="w-4 h-4" /> Restart Chat
                </Button>
            </footer>
        </div>
    );
}
