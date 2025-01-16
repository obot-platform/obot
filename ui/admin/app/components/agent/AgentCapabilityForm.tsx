import { useEffect, useMemo } from "react";
import { useFieldArray, useForm } from "react-hook-form";

import { Agent } from "~/lib/model/agents";

import { ToolEntry } from "~/components/agent/ToolEntry";
import { Switch } from "~/components/ui/switch";
import { useCapabilityTools } from "~/hooks/tools/useCapabilityTools";

type AgentCapabilityFormProps = {
	agent: Agent;
	onChange: (agent: Partial<Agent>) => void;
};

type Item = { tool: string; enabled: boolean };

export function AgentCapabilityForm({
	agent,
	onChange,
}: AgentCapabilityFormProps) {
	const { data: toolReferences } = useCapabilityTools();

	const defaultData = useMemo(() => {
		const capabilities = toolReferences.map((tool) => ({
			tool: tool.id,
			enabled: !!agent.tools?.includes(tool.id),
		}));

		return { capabilities };
	}, [toolReferences, agent.tools]);

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

					if (checked) {
						onChange({ tools: [...(agent.tools ?? []), field.tool] });
					} else {
						onChange({
							tools: (agent.tools ?? []).filter((t) => t !== field.tool),
						});
					}
				}}
			/>
		);
	}
}
