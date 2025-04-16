import { useMemo } from "react";

import { Agent } from "~/lib/model/agents";
import { CapabilityTool } from "~/lib/model/toolReferences";

import { ToolEntry } from "~/components/agent/ToolEntry";
import { ToolFormValues, ToolVariant } from "~/components/agent/ToolForm";
import { getCapabilityToolOrder } from "~/components/agent/shared/constants";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "~/components/ui/select";
import { useCapabilityTools } from "~/hooks/tools/useCapabilityTools";

type AgentCapabilityFormProps = {
	entity: Agent;
	onChange?: (values: ToolFormValues) => void;
	exclude?: CapabilityTool[];
};

export function AgentCapabilityForm({
	entity,
	onChange,
	exclude = [],
}: AgentCapabilityFormProps) {
	const { data: toolReferences } = useCapabilityTools();

	const capabilities = toolReferences
		.filter(
			(tool) =>
				!(exclude as string[]).includes(tool.id) ||
				entity.tools?.includes(tool.id)
		)
		.toSorted(
			(a, b) => getCapabilityToolOrder(a.id) - getCapabilityToolOrder(b.id)
		);

	const values = useMemo(() => {
		return [
			...(entity.tools ?? []).map((tool) => ({
				tool,
				variant: ToolVariant.FIXED,
			})),
			...(entity.defaultThreadTools ?? []).map((tool) => ({
				tool,
				variant: ToolVariant.DEFAULT,
			})),
			...(entity.availableThreadTools ?? []).map((tool) => ({
				tool,
				variant: ToolVariant.AVAILABLE,
			})),
		];
	}, [entity]);

	const valuesMap = new Map<string, ToolVariant>(
		values.map((item) => [item.tool, item.variant])
	);

	const handleChange = (tool: string, value: ToolVariant) => {
		let updatedValues = values;
		if (!valuesMap.has(tool) && value !== ToolVariant.OFF) {
			updatedValues.push({ tool, variant: value });
			onChange?.({
				tools: updatedValues,
				oauthApps: entity.oauthApps ?? [],
			});
			return;
		}

		if (value === ToolVariant.OFF) {
			updatedValues = updatedValues.filter((t) => t.tool !== tool);
		} else {
			const matchingIndex = updatedValues.findIndex((t) => t.tool === tool);

			if (matchingIndex === -1) {
				return;
			}

			updatedValues[matchingIndex].variant = value;
		}

		onChange?.({
			tools: updatedValues,
			oauthApps: entity.oauthApps ?? [],
		});
	};

	return (
		<div>
			{capabilities.map((capability) => (
				<ToolEntry
					key={capability.id}
					withDescription
					tool={capability.id}
					actions={
						<Select
							value={valuesMap.get(capability.id) ?? ToolVariant.OFF}
							onValueChange={(value: ToolVariant) => {
								handleChange(capability.id, value);
							}}
						>
							<SelectTrigger className="w-36">
								<SelectValue />
							</SelectTrigger>

							<SelectContent>
								<SelectItem value={ToolVariant.FIXED}>Always On</SelectItem>
								<SelectItem value={ToolVariant.DEFAULT}>
									<p>
										Optional
										<span className="text-muted-foreground">{" - On"}</span>
									</p>
								</SelectItem>
								<SelectItem value={ToolVariant.AVAILABLE}>
									<p>
										Optional
										<span className="text-muted-foreground">{" - Off"}</span>
									</p>
								</SelectItem>
								<SelectItem value={ToolVariant.OFF}>Off</SelectItem>
							</SelectContent>
						</Select>
					}
				/>
			))}
		</div>
	);
}
