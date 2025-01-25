import { zodResolver } from "@hookform/resolvers/zod";
import { BrainIcon } from "lucide-react";
import { useEffect } from "react";
import { useForm } from "react-hook-form";
import { z } from "zod";

import {
	ControlledAutosizeTextarea,
	ControlledInput,
} from "~/components/form/controlledInputs";
import { CardDescription } from "~/components/ui/card";
import { Form } from "~/components/ui/form";

const formSchema = z.object({
	name: z.string().min(1, {
		message: "Name is required.",
	}),
	description: z.string().optional(),
	prompt: z.string().optional(),
	model: z.string().optional(),
});

export type AgentInfoFormValues = z.infer<typeof formSchema>;

type AgentFormProps = {
	agent: AgentInfoFormValues;
	onSubmit?: (values: AgentInfoFormValues) => void;
	onChange?: (values: AgentInfoFormValues) => void;
};

export function AgentForm({ agent, onSubmit, onChange }: AgentFormProps) {
	const form = useForm<AgentInfoFormValues>({
		resolver: zodResolver(formSchema),
		mode: "onChange",
		defaultValues: {
			name: agent.name || "",
			description: agent.description || "",
			prompt: agent.prompt || "",
			model: agent.model || "",
		},
	});

	useEffect(() => {
		if (agent) form.reset(agent);
	}, [agent, form]);

	useEffect(() => {
		return form.watch((values) => {
			if (!onChange) return;

			const { data, success } = formSchema.safeParse(values);

			if (!success) return;

			onChange(data);
		}).unsubscribe;
	}, [onChange, form]);

	const handleSubmit = form.handleSubmit((values: AgentInfoFormValues) =>
		onSubmit?.({ ...agent, ...values })
	);

	return (
		<Form {...form}>
			<form onSubmit={handleSubmit} className="space-y-4">
				<ControlledInput
					variant="ghost"
					autoComplete="off"
					control={form.control}
					name="name"
					className="text-3xl"
				/>

				<ControlledInput
					variant="ghost"
					control={form.control}
					autoComplete="off"
					name="description"
					placeholder="Add a description..."
					className="text-xl text-muted-foreground"
				/>

				<h4 className="flex items-center gap-2 border-b pb-2">
					<BrainIcon className="h-5 w-5" />
					Instructions
				</h4>

				<CardDescription>
					Give the agent instructions on how to behave and respond to input.
				</CardDescription>

				<ControlledAutosizeTextarea
					control={form.control}
					autoComplete="off"
					name="prompt"
					maxHeight={300}
				/>
			</form>
		</Form>
	);
}
