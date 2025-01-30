import { useEffect, useMemo } from "react";
import { useForm } from "react-hook-form";
import useSWR from "swr";

import { ModelUsage } from "~/lib/model/models";
import { ModelApiService } from "~/lib/service/api/modelApiService";

import { ComboBox } from "~/components/composed/ComboBox";
import { ControlledCustomInput } from "~/components/form/controlledInputs";
import { getModelOptionsByModelProvider } from "~/components/model/DefaultModelAliasForm";
import { Form } from "~/components/ui/form";

type AgentModelSelectProps = {
	entity: { model?: string };
	onChange: (value: { model?: string }) => void;
};

export function AgentModelSelect({ entity, onChange }: AgentModelSelectProps) {
	const getModels = useSWR(
		ModelApiService.getModels.key(),
		ModelApiService.getModels
	);

	const form = useForm({
		defaultValues: { model: entity.model },
	});

	const { reset, watch } = form;

	const models = useMemo(() => {
		if (!getModels.data) return [];

		return getModels.data.filter((m) => !m.usage || m.usage === ModelUsage.LLM);
	}, [getModels.data]);

	useEffect(() => {
		const subscription = watch(onChange);

		return () => subscription.unsubscribe();
	}, [onChange, watch]);

	useEffect(() => {
		reset({ model: entity.model });
	}, [entity.model, reset]);

	const modelOptionsByGroup = getModelOptionsByModelProvider(models);

	return (
		<Form {...form}>
			<ControlledCustomInput control={form.control} name="model">
				{({ field }) => (
					<ComboBox
						allowClear
						clearLabel="Use System Default"
						placeholder="Use System Default"
						value={models.find((m) => m.id === field.value)}
						onChange={(value) => field.onChange(value?.id ?? "")}
						options={modelOptionsByGroup}
					/>
				)}
			</ControlledCustomInput>
		</Form>
	);
}
