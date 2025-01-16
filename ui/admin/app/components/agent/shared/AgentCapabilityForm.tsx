import { useEffect, useMemo } from "react";
import { useFieldArray, useForm } from "react-hook-form";

import { ToolEntry } from "~/components/agent/ToolEntry";
import { getCapabilityToolOrder } from "~/components/agent/shared/constants";
import { Switch } from "~/components/ui/switch";
import { useCapabilityTools } from "~/hooks/tools/useCapabilityTools";

type AgentCapabilityFormProps = {
	entity: { tools?: string[] };
	onChange: (entity: { tools: string[] }) => void;
};

type Item = { tool: string; enabled: boolean };

export function AgentCapabilityForm({
	entity,
	onChange,
}: AgentCapabilityFormProps) {
	const { data: toolReferences } = useCapabilityTools();

	const defaultData = useMemo(() => {
		const capabilities = toolReferences.map((tool) => ({
			tool: tool.id,
			enabled: !!entity.tools?.includes(tool.id),
		}));

		capabilities.sort(
			(a, b) => getCapabilityToolOrder(a.tool) - getCapabilityToolOrder(b.tool)
		);

		return { capabilities };
	}, [toolReferences, entity.tools]);

	const { control, reset } = useForm<{ capabilities: Item[] }>({
		defaultValues: defaultData,
	});

	useEffect(() => {
		reset(defaultData);
	}, [defaultData, reset]);

	const { fields, update } = useFieldArray({
		control,
		name: "capabilities",
	});

	return (
		<div>
			{fields.map((field, index) => (
				<ToolEntry
					key={field.tool}
					tool={field.tool}
					actions={renderActions(field, index)}
				/>
			))}
		</div>
	);

	function renderActions(field: Item, index: number) {
		return (
			<Switch
				checked={field.enabled}
				onCheckedChange={(checked) => {
					update(index, { ...field, enabled: checked });

					// filter early to prevent duplicate entries
					const filtered = (entity.tools ?? []).filter((t) => t !== field.tool);

					if (checked) {
						filtered.push(field.tool);
					}

					onChange({ tools: filtered });
				}}
			/>
		);
	}
}
