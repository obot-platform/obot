import { ArrowRightIcon } from "lucide-react";
import { useMemo } from "react";
import { useNavigate } from "react-router";
import { $path } from "safe-routes";
import useSWR from "swr";

import { Agent } from "~/lib/model/agents";
import { ConsumptionUrl } from "~/lib/routers/baseRouter";
import { AssistantApiService } from "~/lib/service/api/assistantApiService";

import { AgentAccessControl } from "~/components/agent/AgentAccessControl";
import { DeleteAgent } from "~/components/agent/DeleteAgent";
import { Publish } from "~/components/agent/Publish";
import { CopyText } from "~/components/composed/CopyText";
import { WarningAlert } from "~/components/composed/WarningAlert";
import { LoadingSpinner } from "~/components/ui/LoadingSpinner";
import { AnimateResize } from "~/components/ui/animate/animate-resize";
import { Link } from "~/components/ui/link";

type AgentAliasProps = {
	agent: Agent;
	onChange: (agent: Partial<Agent>) => void;
};

export function AgentAlias({ agent, onChange }: AgentAliasProps) {
	const navigate = useNavigate();

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
		`/${!conflictingAlias && agent.alias && agent.aliasAssigned ? agent.alias : agent.id}`
	);

	return (
		<div className="sticky top-0 z-10 flex h-16 w-full flex-col gap-4 border-b bg-background px-8 pt-4">
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
								as="button"
								to={agentUrl}
								target="_blank"
								rel="noreferrer"
								className="group flex items-center gap-2"
							>
								<AnimateResize>
									<span className="group-hover:hidden">Try it Out!</span>
									<span className="hidden group-hover:block">{agentUrl}</span>
								</AnimateResize>
								<ArrowRightIcon />
							</Link>
						</div>
					)}
				</div>

				<div className="flex gap-2">
					<Publish
						alias={agent.alias}
						id={agent.id}
						onPublish={(alias) => onChange({ alias })}
					/>
					<AgentAccessControl agent={agent} />
					<DeleteAgent id={agent.id} onSuccess={() => navigate("/agents")} />
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
										? $path("/agents/:id", {
												id: refAssistant.entityID,
											})
										: $path("/tasks/:id", {
												id: refAssistant.entityID,
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
