import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect } from "react";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { Step, Task } from "~/lib/model/tasks";
import { noop } from "~/lib/utils";

import { AddStepButton } from "~/components/task/steps/AddStep";
import { StepBase } from "~/components/task/steps/StepBase";
import { Animate, AnimatePresence } from "~/components/ui/animate";
import { SortableList } from "~/components/ui/dnd/sortable";
import { Form, FormField, FormItem, FormMessage } from "~/components/ui/form";

const formSchema = z.object({
	steps: z.array(z.custom<Step>()),
});

export type StepsFormValues = z.infer<typeof formSchema>;

export function StepsForm({
	task,
	onSubmit,
	onChange,
}: {
	task: Task;
	onSubmit?: (values: StepsFormValues) => void;
	onChange?: (values: StepsFormValues) => void;
}) {
	const form = useForm<StepsFormValues>({
		resolver: zodResolver(formSchema),
		defaultValues: { steps: task.steps || [] },
	});

	const handleSubmit = form.handleSubmit(onSubmit || noop);

	const stepValues = form.watch("steps");

	useEffect(() => {
		form.reset({ steps: task.steps || [] });
	}, [task, form]);

	useEffect(() => {
		return form.watch((values) => {
			const { data, success } = formSchema.safeParse(values);
			if (!success) return;
			onChange?.(data);
		}).unsubscribe;
	}, [stepValues, form.formState, onChange, form]);

	return (
		<Form {...form}>
			<AnimatePresence>
				<Animate.form onSubmit={handleSubmit} layout="size">
					<FormField
						control={form.control}
						name="steps"
						render={({ field }) => (
							<FormItem>
								<SortableList
									items={field.value}
									getKey={(step) => step.id}
									isHandle={false}
									onChange={field.onChange}
									renderItem={(step, index) => {
										const onUpdate = (updatedStep: Step) => {
											const newSteps = [...field.value];
											newSteps[index] = updatedStep;
											field.onChange(newSteps);
										};

										const onDelete = () => {
											const newSteps = field.value.filter(
												(_, i) => i !== index
											);
											field.onChange(newSteps);
										};

										return (
											<StepBase
												key={step.id}
												step={step}
												onUpdate={onUpdate}
												onDelete={onDelete}
											/>
										);
									}}
								/>

								<AddStepButton
									className="float-end"
									onAddStep={(newStep) => {
										field.onChange([...field.value, newStep]);
									}}
								/>
								<FormMessage />
							</FormItem>
						)}
					/>
				</Animate.form>
			</AnimatePresence>
		</Form>
	);
}
