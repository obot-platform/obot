import { useMemo } from "react";
import useSWR from "swr";

import { isCapabilityTool } from "~/lib/model/toolReferences";
import { ToolReferenceService } from "~/lib/service/api/toolreferenceService";

export function useCapabilityTools() {
	const { data: toolReferences, ...rest } = useSWR(
		ToolReferenceService.getToolReferences.key("tool"),
		({ type }) => ToolReferenceService.getToolReferences(type),
		{ fallbackData: [] }
	);

	const filtered = useMemo(
		() => toolReferences.filter(isCapabilityTool),
		[toolReferences]
	);

	return { data: filtered, ...rest };
}
