import { ReactNode, memo, useCallback, useMemo, useState } from "react";
import useSWR from "swr";

import { ToolReferenceService } from "~/lib/service/api/toolreferenceService";

import { ToolEntry } from "~/components/agent/ToolEntry";
import { ToolCatalogDialog } from "~/components/tools/ToolCatalog";
import { useCapabilityTools } from "~/hooks/tools/useCapabilityTools";

export const BasicToolForm = memo(function BasicToolFormComponent(props: {
	value?: string[];
	defaultValue?: string[];
	oauths?: string[];
	onChange?: (values: string[], toolOauths?: string[]) => void;
	renderActions?: (tool: string) => ReactNode;
}) {
	const { onChange, renderActions, oauths } = props;
	const { data: toolList } = useSWR(
		ToolReferenceService.getToolReferences.key("tool"),
		() => ToolReferenceService.getToolReferences("tool"),
		{ fallbackData: [] }
	);

	const oauthToolMap = useMemo(
		() => new Map(toolList.map((tool) => [tool.id, tool.metadata?.oauth])),
		[toolList]
	);

	const [_value, _setValue] = useState(props.defaultValue);
	const value = useMemo(
		() => props.value ?? _value ?? [],
		[props.value, _value]
	);

	const setValue = useCallback(
		(newValue: string[], toolOauths?: string[]) => {
			_setValue(newValue);
			onChange?.(newValue, toolOauths);
		},
		[onChange]
	);

	const getCapabilities = useCapabilityTools();
	const capabilities = new Set(getCapabilities.data?.map((tool) => tool.id));
	const filtered = value.filter((tool) => !capabilities.has(tool));

	const removeTool = (toolId: string, oauthToRemove?: string) => {
		const updatedTools = value.filter((tool) => tool !== toolId);
		const stillHasOauth = updatedTools.some(
			(tool) => oauthToolMap.get(tool) === oauthToRemove
		);
		const updatedOauths = stillHasOauth
			? oauths
			: oauths?.filter((oauth) => oauth !== oauthToRemove);
		setValue(updatedTools, updatedOauths);
	};

	return (
		<div className="flex flex-col gap-2">
			<div className="flex w-full flex-col gap-1 overflow-y-auto">
				{filtered.map((tool) => (
					<ToolEntry
						key={tool}
						tool={tool}
						onDelete={removeTool}
						actions={renderActions?.(tool)}
					/>
				))}
			</div>

			<div className="flex justify-end">
				<ToolCatalogDialog
					tools={value}
					onUpdateTools={setValue}
					oauths={oauths ?? []}
				/>
			</div>
		</div>
	);
});
