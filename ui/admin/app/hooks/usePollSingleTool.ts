import { useEffect, useRef, useState } from "react";
import useSWR from "swr";

import { ToolReferenceService } from "~/lib/service/api/toolreferenceService";

export function usePollSingleTool(toolId: string) {
	const [isPolling, setIsPolling] = useState(false);
	const isInitial = useRef(false);

	const { mutate: updateTools } = useSWR(
		isPolling ? ToolReferenceService.getToolReferences.key("tool") : null,
		({ type }) => ToolReferenceService.getToolReferences(type),
		{ fallbackData: [], revalidateIfStale: false }
	);

	const getTool = useSWR(
		isPolling ? ToolReferenceService.getToolReferenceById.key(toolId) : null,
		({ toolReferenceId }) =>
			ToolReferenceService.getToolReferenceById(toolReferenceId),
		{ refreshInterval: 1000 }
	);

	useEffect(() => {
		// skip initial poll in case data is stale
		// stale data could give a false positive on the `resolved` property
		// which would prematurely stop polling
		if (isInitial.current) {
			isInitial.current = false;
			return;
		}

		if (!getTool.data) return;

		setIsPolling(!getTool.data.resolved);

		// resolved means async update is complete
		if (getTool.data.resolved) {
			updateTools(
				(tools) => {
					if (!getTool.data) return tools;
					if (!tools) return [getTool.data];

					const index = tools.findIndex((tool) => tool.id === toolId);

					const copy = [...tools];
					copy[index] = getTool.data;
					return copy;
				},
				{ revalidate: false }
			);
		}
	}, [getTool.data, updateTools, toolId]);

	return {
		startPolling: () => {
			isInitial.current = true;
			setIsPolling(true);
		},
		isPolling,
	};
}
