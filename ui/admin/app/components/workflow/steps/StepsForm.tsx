import { zodResolver } from "@hookform/resolvers/zod";
import { AnimatePresence, motion } from "motion/react";
import { useEffect } from "react";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { Step, Workflow } from "~/lib/model/workflows";
import { noop } from "~/lib/utils";

import { SortableList } from "~/components/ui/dnd/sortable";
import { Form, FormField, FormItem, FormMessage } from "~/components/ui/form";
import { AddStepButton } from "~/components/workflow/steps/AddStep";
import { renderStep } from "~/components/workflow/steps/StepRenderer";

const formSchema = z.object({
	steps: z.array(z.custom<Step>()),
});

export type StepsFormValues = z.infer<typeof formSchema>;

export function StepsForm({
	workflow,
	onSubmit,
	onChange,
}: {
	workflow: Workflow;
	onSubmit?: (values: StepsFormValues) => void;
	onChange?: (values: StepsFormValues) => void;
}) {
	const form = useForm<StepsFormValues>({
		resolver: zodResolver(formSchema),
		defaultValues: { steps: workflow.steps || [] },
	});

	const handleSubmit = form.handleSubmit(onSubmit || noop);

	const stepValues = form.watch("steps");

	useEffect(() => {
		console.log(workflow.steps);

		form.reset({ steps: workflow.steps || [] });
	}, [workflow, form]);

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
				<motion.form onSubmit={handleSubmit} layout="size">
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
											console.log(updatedStep);

											newSteps[index] = updatedStep;
											field.onChange(newSteps);
										};

										const onDelete = () => {
											const newSteps = field.value.filter(
												(_, i) => i !== index
											);
											field.onChange(newSteps);
										};

										return renderStep({
											step,
											onUpdate,
											onDelete,
										});
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
				</motion.form>
			</AnimatePresence>
		</Form>
	);
}
