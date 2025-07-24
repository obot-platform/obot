import { useMemo } from "react";
import useSWR from "swr";

import { ModelUsage } from "~/lib/model/models";
import { ModelApiService } from "~/lib/service/api/modelApiService";

import { MultiSelect, type Option } from "~/components/ui/multi-select";

type AgentAllowedModelsSelectProps = {
	entity: { allowedModels?: string[] };
	onChange: (value: { allowedModels?: string[] }) => void;
};

export function AgentAllowedModelsSelect({
	entity,
	onChange,
}: AgentAllowedModelsSelectProps) {
	const { data: models = [] } = useSWR(
		ModelApiService.getModels.key(),
		ModelApiService.getModels
	);

	// Filter to only show LLM models
	const llmModels = useMemo(() => {
		return models.filter((m) => !m.usage || m.usage === ModelUsage.LLM);
	}, [models]);

	// Convert models to MultiSelect options
	const modelOptions = useMemo<Option[]>(() => {
		return llmModels.map((model) => ({
			value: model.name || model.id,
			label: model.name || model.id,
		}));
	}, [llmModels]);

	// Convert selected model names to MultiSelect options
	const selectedOptions = useMemo<Option[]>(() => {
		return (entity.allowedModels || [])
			.map((modelName) => {
				const model = llmModels.find((m) => (m.name || m.id) === modelName);
				return model
					? {
							value: model.name || model.id,
							label: model.name || model.id,
						}
					: null;
			})
			.filter((option): option is Option => option !== null);
	}, [entity.allowedModels, llmModels]);

	const handleSelectionChange = (options: Option[]) => {
		const selectedModelNames = options.map((option) => option.value);
		onChange({ allowedModels: selectedModelNames });
	};

	return (
		<div className="space-y-4">
			<MultiSelect
				value={selectedOptions}
				options={modelOptions}
				onChange={handleSelectionChange}
				placeholder="Select allowed models..."
				emptyIndicator="No models available"
				className="w-full"
			/>
		</div>
	);
}
