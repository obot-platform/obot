import { useEffect, useState } from "react";
import useSWR from "swr";

import { ToolInfo } from "~/lib/model/agents";
import { AssistantNamespace } from "~/lib/model/assistants";
import { AgentService } from "~/lib/service/api/agentService";

export function useToolAuthPolling(
	namespace: AssistantNamespace,
	entityId?: Nullish<string>
) {
	const [toolInfo, setToolInfo] = useState<Record<string, ToolInfo> | null>(
		null
	);
	const [isPolling, setIsPolling] = useState(false);
	const refreshInterval = isPolling ? 1000 : undefined;

	const [key, handler] = AgentService.getAgentById.swr({ agentId: entityId });

	const { data: agent } = useSWR(
		namespace === AssistantNamespace.Agents ? key : null,
		handler,
		{ refreshInterval }
	);

	const refresh = () => {
		setIsPolling(true);
	};

	useEffect(() => {
		const getInfo = () => {
			const agentTools = [
				...(agent?.tools ?? []),
				...(agent?.availableThreadTools ?? []),
				...(agent?.defaultThreadTools ?? []),
			];

			switch (namespace) {
				case AssistantNamespace.Agents:
					return { tools: agentTools, toolInfo: agent?.toolInfo };
				default:
					return {};
			}
		};

		const { tools, toolInfo: toolInfoFromAgent } = getInfo();
		if (toolInfoFromAgent) setToolInfo(toolInfoFromAgent);

		// when tool credentials are processing the api will respond with { toolInfo: null }
		// we need to poll until the toolInfo is not null or there are no tools
		const shouldPoll = !!tools?.length && !toolInfoFromAgent;
		if (shouldPoll !== isPolling) setIsPolling(shouldPoll);
	}, [agent, isPolling, namespace]);

	return { toolInfo, setToolInfo, refresh };
}
