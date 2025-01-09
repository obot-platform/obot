import { useMemo } from "react";
import { Link } from "react-router";
import { $path } from "safe-routes";
import useSWR from "swr";

import { Agent } from "~/lib/model/agents";
import { ConsumptionUrl } from "~/lib/routers/baseRouter";
import { AssistantApiService } from "~/lib/service/api/assistantApiService";

import { AgentAccessControl } from "~/components/agent/AgentAccessControl";
import { AgentDropdownActions } from "~/components/agent/AgentDropdownActions";
import { Publish } from "~/components/agent/Publish";
import { CopyText } from "~/components/composed/CopyText";
import { WarningAlert } from "~/components/composed/WarningAlert";
import { LoadingSpinner } from "~/components/ui/LoadingSpinner";

type AgentAliasProps = {
	agent: Agent;
	onChange: (agent: Partial<Agent>) => void;
};

export function AgentAlias({ agent, onChange }: AgentAliasProps) {
	const getAssistants = useSWR(
		() => AssistantApiService.getAssistants.key(),
		() => AssistantApiService.getAssistants()
	);

	const refAssistant = useMemo(() => {
		if (!getAssistants.data) return null;

		return getAssistants.data.find(({ id }) => id === agent.alias);
	}, [getAssistants.data, agent.alias]);

	const conflictingAlias = refAssistant && refAssistant.entityID !== agent.id;
	const agentUrl = ConsumptionUrl(
		`/${!conflictingAlias && agent.alias ? agent.alias : agent.id}`
	);

	return (
		<div className="flex w-full flex-col gap-4 px-8 pt-4">
			<div className="flex w-full justify-between gap-4">
				<div className="flex flex-col gap-2">
					{agent.aliasAssigned === undefined &&
					agent.alias &&
					!conflictingAlias ? (
						<LoadingSpinner className="m-l-2 text-muted-foreground" />
					) : (
						<div className="flex items-center gap-2">
							<CopyText
								className="h-8 flex-row-reverse bg-background text-sm text-muted-foreground"
								holdStatusDelay={6000}
								text={agentUrl}
								hideText
							/>

							<Link
								target="_blank"
								rel="noreferrer"
								className="text-muted-foreground underline"
								to={agentUrl}
							>
								{agentUrl}
							</Link>

							<Publish
								alias={agent.alias}
								id={agent.id}
								onPublish={(alias) => onChange({ alias })}
							/>
						</div>
					)}
				</div>

				<div className="flex gap-2">
					<AgentAccessControl agent={agent} />
					<AgentDropdownActions agent={agent} />
				</div>
			</div>
			{conflictingAlias && (
				<WarningAlert
					title="Alias Already In Use!"
					description={
						<small className="pb-0 text-muted-foreground">
							<span className="min-w-fit">
								Defaulting to non-alias URL, <b>{refAssistant.id}</b> is taken
								by{" "}
							</span>
							<Link
								className="text-accent-foreground underline"
								to={
									refAssistant.type === "agent"
										? $path("/agents/:agent", {
												agent: refAssistant.entityID,
											})
										: $path("/workflows/:workflow", {
												workflow: refAssistant.entityID,
											})
								}
							>
								{refAssistant.name}
							</Link>
						</small>
					}
				/>
			)}
		</div>
	);
}
