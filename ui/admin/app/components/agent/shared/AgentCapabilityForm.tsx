import { CapabilityTool } from "~/lib/model/toolReferences";

import { ToolEntry } from "~/components/agent/ToolEntry";
import { getCapabilityToolOrder } from "~/components/agent/shared/constants";
import { ClickableDiv } from "~/components/ui/clickable-div";
import { Switch } from "~/components/ui/switch";
import { useCapabilityTools } from "~/hooks/tools/useCapabilityTools";

type AgentCapabilityFormProps = {
	entity: { tools?: string[] };
	onChange: (entity: { tools: string[] }) => void;
	exclude?: CapabilityTool[];
};

export function AgentCapabilityForm({
	entity,
	onChange,
	exclude = [],
}: AgentCapabilityFormProps) {
	const { data: toolReferences } = useCapabilityTools();

	const currentTools = new Set(entity.tools ?? []);

	const capabilities = toolReferences
		.filter(
			(tool) =>
				!(exclude as string[]).includes(tool.id) ||
				entity.tools?.includes(tool.id)
		)
		.toSorted(
			(a, b) => getCapabilityToolOrder(a.id) - getCapabilityToolOrder(b.id)
		);

	const handleToggle = (id: string) => {
		const filtered = (entity.tools ?? []).filter((t) => t !== id);

		if (!currentTools.has(id)) {
			filtered.push(id);
		}

		onChange({ tools: filtered });
	};

	return (
		<div>
			{capabilities.map((capability) => (
				<ClickableDiv
					key={capability.id}
					onClick={() => handleToggle(capability.id)}
				>
					<ToolEntry
						withDescription
						tool={capability.id}
						actions={<Switch checked={currentTools.has(capability.id)} />}
					/>
				</ClickableDiv>
			))}
		</div>
	);
}
