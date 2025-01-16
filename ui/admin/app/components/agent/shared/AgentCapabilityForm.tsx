import { ToolReference } from "~/lib/model/toolReferences";

import { ToolEntry } from "~/components/agent/ToolEntry";
import { getCapabilityToolOrder } from "~/components/agent/shared/constants";
import { Switch } from "~/components/ui/switch";
import { useCapabilityTools } from "~/hooks/tools/useCapabilityTools";

type AgentCapabilityFormProps = {
	entity: { tools?: string[] };
	onChange: (entity: { tools: string[] }) => void;
};

export function AgentCapabilityForm({
	entity,
	onChange,
}: AgentCapabilityFormProps) {
	const { data: toolReferences } = useCapabilityTools();

	const addedTools = new Set(entity.tools ?? []);

	const capabilities = toolReferences.toSorted(
		(a, b) => getCapabilityToolOrder(a.id) - getCapabilityToolOrder(b.id)
	);

	return (
		<div>
			{capabilities.map((capability) => (
				<ToolEntry
					withDescription
					key={capability.id}
					tool={capability.id}
					actions={renderActions(capability)}
				/>
			))}
		</div>
	);

	function renderActions(capability: ToolReference) {
		return (
			<Switch
				checked={addedTools.has(capability.id)}
				onCheckedChange={(checked) => {
					// always filter tool out to prevent duplicate entries
					const filtered = (entity.tools ?? []).filter(
						(t) => t !== capability.id
					);

					if (checked) {
						filtered.push(capability.id);
					}

					onChange({ tools: filtered });
				}}
			/>
		);
	}
}
