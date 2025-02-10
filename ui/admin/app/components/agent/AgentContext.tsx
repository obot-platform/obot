import {
	ReactNode,
	createContext,
	useContext,
	useEffect,
	useState,
} from "react";
import { toast } from "sonner";
import useSWR from "swr";

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

	const getAgent = useSWR(...AgentService.getAgentById.swr({ agentId }), {
		fallbackData: agent,
		refreshInterval: blockPollingAgent ? undefined : 1000,
	});

	const agentData = getAgent.data ?? agent;

	useEffect(() => {
		if (agentData?.alias && agentData.aliasAssigned === undefined) {
			setBlockPollingAgent(false);
		} else {
			setBlockPollingAgent(true);
		}
	}, [agentData]);

	const updateAgent = useAsync(
		async (updatedAgent: Agent) =>
			AgentService.updateAgent(
				{ id: agentId, agent: updatedAgent },
				{ cancellable: true }
			),
		{
			onSuccess: (updatedAgent) => {
				getAgent.mutate(updatedAgent);
				AgentService.getAgents.revalidate();
			},
			onError: () => toast.error(`Error saving agent`),
		}
	);

	const refreshAgent = getAgent.mutate;

	return (
		<AgentContext.Provider
			value={{
				agentId,
				agent: agentData,
				updateAgent: updateAgent.executeAsync,
				refreshAgent,
				isUpdating: updateAgent.isLoading,
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
