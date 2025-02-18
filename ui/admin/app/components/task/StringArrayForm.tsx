import { zodResolver } from "@hookform/resolvers/zod";
import { PlusIcon, TrashIcon } from "lucide-react";
import { useEffect, useMemo } from "react";
import { useFieldArray, useForm } from "react-hook-form";
import { z } from "zod";

import { ControlledInput } from "~/components/form/controlledInputs";
import { Button } from "~/components/ui/button";
import { Form } from "~/components/ui/form";

const formSchema = z.object({
	items: z.array(z.object({ value: z.string() })),
});

export type StringArrayFormValues = z.infer<typeof formSchema>;

export function StringArrayForm({
	initialItems,
	onSubmit,
	onChange,
	itemName,
	placeholder,
}: {
	initialItems?: string[];
	onSubmit?: (values: string[]) => void;
	onChange?: (values: string[]) => void;
	itemName: string;
	placeholder: string;
}) {
	const defaultValues = useMemo(
		() => convertToFormValues(initialItems ?? []),
		[initialItems]
	);

	const form = useForm<StringArrayFormValues>({
		resolver: zodResolver(formSchema),
		defaultValues,
	});

	const handleSubmit = form.handleSubmit((values) => {
		if (!onSubmit) return;

		onSubmit(convertFromFormValues(values));
	});

	useEffect(() => {
		if (!onChange) return;

		return form.watch((values) => {
			const { data, success } = formSchema.safeParse(values);
			if (!success) return;
			onChange(convertFromFormValues(data));
		}).unsubscribe;
	}, [form.formState, onChange, form]);

	const items = useFieldArray({
		control: form.control,
		name: "items",
	});

	return (
		<Form {...form}>
			<form onSubmit={handleSubmit} className="flex flex-col gap-2">
				{items.fields.map((field, index) => (
					<div key={field.id} className="flex gap-2 rounded-md bg-muted p-2">
						<ControlledInput
							classNames={{
								wrapper: "flex-grow",
								input: "bg-background",
							}}
							control={form.control}
							name={`items.${index}.value`}
							placeholder={placeholder}
						/>

						<Button
							size="icon"
							variant="ghost"
							onClick={() => items.remove(index)}
							startContent={<TrashIcon />}
						/>
					</div>
				))}

				<Button
					type="button"
					className="self-end"
					variant="ghost"
					onClick={() => items.append({ value: "" })}
					startContent={<PlusIcon />}
				>
					Add {itemName}
				</Button>
			</form>
		</Form>
	);
}

function convertToFormValues(items: string[]) {
	return { items: items.map((item) => ({ value: item })) };
}

function convertFromFormValues(values: StringArrayFormValues) {
	return values.items.map((item) => item.value);
}
