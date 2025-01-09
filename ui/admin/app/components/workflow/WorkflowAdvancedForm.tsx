import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect } from "react";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { Workflow } from "~/lib/model/workflows";

import {
	Form,
	FormControl,
	FormField,
	FormItem,
	FormLabel,
	FormMessage,
} from "~/components/ui/form";
import { Textarea } from "~/components/ui/textarea";

const formSchema = z.object({
	prompt: z.string().optional(),
	output: z.string().optional(),
});

export type WorkflowAdvancedFormValues = z.infer<typeof formSchema>;

type WorkflowAdvancedFormProps = {
	workflow: Workflow;
	onSubmit?: (values: WorkflowAdvancedFormValues) => void;
	onChange?: (values: WorkflowAdvancedFormValues) => void;
};

export function WorkflowAdvancedForm({
	workflow,
	onSubmit,
	onChange,
}: WorkflowAdvancedFormProps) {
	const form = useForm<WorkflowAdvancedFormValues>({
		resolver: zodResolver(formSchema),
		defaultValues: {
			prompt: workflow.prompt || "",
			output: workflow.output || "",
		},
	});

	useEffect(() => {
		if (workflow) form.reset(workflow);
	}, [workflow, form]);

	useEffect(() => {
		return form.watch((values) => {
			const { data, success } = formSchema.safeParse(values);

			if (!success) return;

			onChange?.(data);
		}).unsubscribe;
	}, [onChange, form]);

	const handleSubmit = form.handleSubmit((values: WorkflowAdvancedFormValues) =>
		onSubmit?.({ ...workflow, ...values })
	);

	return (
		<Form {...form}>
			<form onSubmit={handleSubmit} className="space-y-8">
				<FormField
					control={form.control}
					name="prompt"
					render={({ field }) => (
						<FormItem>
							<FormLabel>Workflow Prompt</FormLabel>

							<FormControl>
								<Textarea
									rows={4}
									placeholder="What should the workflow do?"
									{...field}
								/>
							</FormControl>
							<FormMessage />
						</FormItem>
					)}
				/>
				<FormField
					control={form.control}
					name="output"
					render={({ field }) => (
						<FormItem>
							<FormLabel>Workflow Output</FormLabel>

							<FormControl>
								<Textarea
									rows={4}
									placeholder="What should the workflow output?"
									{...field}
								/>
							</FormControl>
							<FormMessage />
						</FormItem>
					)}
				/>
			</form>
		</Form>
	);
}
