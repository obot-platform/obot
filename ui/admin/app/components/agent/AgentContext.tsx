import {
	ReactNode,
	createContext,
	useCallback,
	useContext,
	useEffect,
	useRef,
	useState,
} from "react";
import useSWR, { mutate } from "swr";

import { Agent } from "~/lib/model/agents";
import { AgentService } from "~/lib/service/api/agentService";

import { useAsync } from "~/hooks/useAsync";

interface AgentContextType {
	agent: Agent;
	agentId: string;
	updateAgent: (agent: Agent) => Promise<unknown>;
	refreshAgent: (agent?: Agent) => Promise<unknown>;
	isUpdating: boolean;
	error?: unknown;
	lastUpdated?: Date;
}

const AgentContext = createContext<AgentContextType | undefined>(undefined);

export function AgentProvider({
	children,
	agent,
}: {
	children: ReactNode;
	agent: Agent;
}) {
	const agentId = agent.id;

	const [blockPollingAgent, setBlockPollingAgent] = useState(false);
	const abortControllerRef = useRef<AbortController>();

	const getAgent = useSWR(
		AgentService.getAgentById.key(agentId),
		({ agentId }) => AgentService.getAgentById(agentId),
		{
			fallbackData: agent,
			refreshInterval: blockPollingAgent ? undefined : 1000,
		}
	);

	const agentData = getAgent.data ?? agent;

	useEffect(() => {
		if (agentData?.alias && agentData.aliasAssigned === undefined) {
			setBlockPollingAgent(false);
		} else {
			setBlockPollingAgent(true);
		}
	}, [agentData]);

	const [lastUpdated, setLastSaved] = useState<Date>();

	const handleUpdateAgent = useCallback(
		(updatedAgent: Agent) => {
			// Abort any ongoing request
			if (abortControllerRef.current) {
				abortControllerRef.current.abort({
					reason: "Superseded by newer save request",
					timestamp: Date.now(),
				});
			}

			// Create new AbortController for this request
			const controller = new AbortController();
			abortControllerRef.current = controller;

			return AgentService.updateAgent({
				id: agentId,
				agent: updatedAgent,
				signal: controller.signal,
			})
				.then((updatedAgent) => {
					if (!controller.signal.aborted) {
						getAgent.mutate(updatedAgent);
						mutate(AgentService.getAgents.key());
						setLastSaved(new Date());
					}
				})
				.catch(console.error)
				.finally(() => {
					abortControllerRef.current = undefined;
				});
		},
		[agentId, getAgent]
	);

	const updateAgent = useAsync(handleUpdateAgent);

	const refreshAgent = getAgent.mutate;

	return (
		<AgentContext.Provider
			value={{
				agentId,
				agent: agentData,
				updateAgent: updateAgent.executeAsync,
				refreshAgent,
				isUpdating: updateAgent.isLoading,
				lastUpdated,
				error: updateAgent.error,
			}}
		>
			{children}
		</AgentContext.Provider>
	);
}

export function useAgent() {
	const context = useContext(AgentContext);
	if (context === undefined) {
		throw new Error("useChat must be used within a ChatProvider");
	}
	return context;
}
