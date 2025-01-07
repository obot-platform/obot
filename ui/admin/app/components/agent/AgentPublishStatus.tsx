import { useMemo } from "react";
import { Link } from "react-router";
import { $path } from "safe-routes";
import useSWR from "swr";

import { Agent } from "~/lib/model/agents";
import { ConsumptionUrl } from "~/lib/routers/baseRouter";
import { AssistantApiService } from "~/lib/service/api/assistantApiService";

import { AgentDropdownActions } from "~/components/agent/AgentDropdownActions";
import { Publish } from "~/components/agent/Publish";
import { Unpublish } from "~/components/agent/Unpublish";
import { CopyText } from "~/components/composed/CopyText";
import { LoadingSpinner } from "~/components/ui/LoadingSpinner";

type AgentPublishStatusProps = {
    agent: Agent;
    onChange: (agent: Partial<Agent>) => void;
};

export function AgentPublishStatus({
    agent,
    onChange,
}: AgentPublishStatusProps) {
    const getAssistants = useSWR(
        () =>
            agent.alias && !agent.aliasAssigned
                ? AssistantApiService.getAssistants.key()
                : null,
        () => AssistantApiService.getAssistants()
    );

    const refAssistant = useMemo(() => {
        if (!getAssistants.data) return null;

        return getAssistants.data.find(({ id }) => id === agent.alias);
    }, [getAssistants.data, agent.alias]);

    return (
        <div className="flex w-full justify-between px-8 pt-4 items-center gap-4">
            {renderAgentRef()}

            <div className="flex items-center gap-2">
                {agent.alias ? (
                    <Unpublish onUnpublish={() => onChange({ alias: "" })} />
                ) : (
                    <Publish
                        alias={agent.alias}
                        onPublish={(alias) => onChange({ alias })}
                    />
                )}

                <AgentDropdownActions agent={agent} />
            </div>
        </div>
    );

    function renderAgentRef() {
        if (!agent.alias) return <div />;

        if (refAssistant && refAssistant.entityID !== agent.id) {
            const route =
                refAssistant.type === "agent"
                    ? $path("/agents/:agent", {
                          agent: refAssistant.entityID,
                      })
                    : $path("/workflows/:workflow", {
                          workflow: refAssistant.entityID,
                      });

            return (
                <div className="flex flex-col gap-1 h-full">
                    <div className="flex items-center gap-2">
                        <div className="size-2 bg-warning rounded-full" />
                        <small>Unavailable</small>
                    </div>

                    <small className="pb-0 text-muted-foreground">
                        <span className="min-w-fit">
                            Ref name <b>{refAssistant.id}</b> used by{" "}
                        </span>
                        <Link
                            className="text-accent-foreground underline"
                            to={route}
                        >
                            {refAssistant.name}
                        </Link>
                    </small>
                </div>
            );
        }

        // if aliasAssigned is undefined, it is still resolving
        if (agent.aliasAssigned === undefined)
            return <LoadingSpinner className="m-l-2 text-muted-foreground" />;

        if (!agent.aliasAssigned) return <div />;

        const agentUrl = ConsumptionUrl(`/${agent.alias}`);

        return (
            <div className="flex items-center gap-2">
                <CopyText
                    className="h-8 text-muted-foreground text-sm bg-background flex-row-reverse"
                    holdStatusDelay={6000}
                    text={agentUrl}
                    iconOnly
                />

                <Link
                    target="_blank"
                    rel="noreferrer"
                    className="text-muted-foreground underline"
                    to={agentUrl}
                >
                    {agentUrl}
                </Link>
            </div>
        );
    }
}
