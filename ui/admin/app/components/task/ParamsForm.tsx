import { ComponentProps, useMemo } from "react";

import { Task } from "~/lib/model/tasks";

import { NameDescriptionForm } from "~/components/composed/NameDescriptionForm";

type ParamValues = Record<string, string>;

const convertFrom = (params: ParamValues) => {
	const converted = Object.entries(params || {}).map(([name, description]) => ({
		name,
		description,
	}));

	return converted.length ? converted : [];
};

const convertTo = (
	params: ComponentProps<typeof NameDescriptionForm>["defaultValues"]
) => {
	if (!params?.length) return undefined;

	return params.reduce((acc, param) => {
		if (!param.name) return acc;

		acc[param.name] = param.description;
		return acc;
	}, {} as NonNullable<ParamValues>);
};

export function ParamsForm({
	task,
	onChange,
}: {
	task: Task;
	onChange?: (values: { params?: ParamValues }) => void;
}) {
	const defaultValues = useMemo(
		() => convertFrom(task.onDemand?.params ?? {}),
		[task.onDemand?.params]
	);

	return (
		<NameDescriptionForm
			asCard
			addLabel="Add Parameter"
			defaultValues={defaultValues}
			onChange={(values) => onChange?.({ params: convertTo(values) })}
		/>
	);
}
