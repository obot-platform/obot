import { ReactNode, memo, useCallback, useMemo, useState } from "react";

import { ToolEntry } from "~/components/agent/ToolEntry";
import { ToolCatalogDialog } from "~/components/tools/ToolCatalog";
import { useCapabilityTools } from "~/hooks/tools/useCapabilityTools";

export const BasicToolForm = memo(function BasicToolFormComponent(props: {
	value?: string[];
	defaultValue?: string[];
	onChange?: (values: string[]) => void;
	renderActions?: (tool: string) => ReactNode;
}) {
	const { onChange, renderActions } = props;

	const [_value, _setValue] = useState(props.defaultValue);
	const value = useMemo(
		() => props.value ?? _value ?? [],
		[props.value, _value]
	);

	const setValue = useCallback(
		(newValue: string[]) => {
			_setValue(newValue);
			onChange?.(newValue);
		},
		[onChange]
	);

	const getCapabilities = useCapabilityTools();
	const capabilities = new Set(getCapabilities.data?.map((tool) => tool.id));
	const filtered = value.filter((tool) => !capabilities.has(tool));

	const removeTools = (toolsToRemove: string[]) => {
		setValue(value.filter((tool) => !toolsToRemove.includes(tool)));
	};

	return (
		<div className="flex flex-col gap-2">
			<div className="flex w-full flex-col gap-1 overflow-y-auto">
				{filtered.map((tool) => (
					<ToolEntry
						key={tool}
						tool={tool}
						onDelete={() => removeTools([tool])}
						actions={renderActions?.(tool)}
					/>
				))}
			</div>

			<div className="flex justify-end">
				<ToolCatalogDialog tools={value} onUpdateTools={setValue} />
			</div>
		</div>
	);
});
