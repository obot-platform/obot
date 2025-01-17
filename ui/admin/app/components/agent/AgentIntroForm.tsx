import { zodResolver } from "@hookform/resolvers/zod";
import { PlusIcon, SmileIcon, TrashIcon } from "lucide-react";
import { useEffect } from "react";
import { useForm } from "react-hook-form";
import { z } from "zod";

import {
	ControlledAutosizeTextarea,
	ControlledInput,
} from "~/components/form/controlledInputs";
import { Button } from "~/components/ui/button";
import { CardDescription } from "~/components/ui/card";
import { Form } from "~/components/ui/form";

const formSchema = z.object({
	introductionMessage: z.string().optional(),
	starterMessages: z.array(z.string()).optional(),
});

export type AgentInfoFormValues = z.infer<typeof formSchema>;

type AgentIntroFormProps = {
	agent: AgentInfoFormValues;
	onSubmit?: (values: AgentInfoFormValues) => void;
	onChange?: (values: AgentInfoFormValues) => void;
};

export function AgentIntroForm({
	agent,
	onChange,
	onSubmit,
}: AgentIntroFormProps) {
	const form = useForm<AgentInfoFormValues>({
		resolver: zodResolver(formSchema),
		mode: "onChange",
		defaultValues: {
			introductionMessage: "",
			starterMessages: [],
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

	const starterMessages = form.watch("starterMessages");
	return (
		<Form {...form}>
			<form onSubmit={handleSubmit} className="space-y-4">
				<h4 className="mb-4 flex items-center gap-2 border-b pb-2">
					<SmileIcon className="h-5 w-5" />
					Introductions
				</h4>

				<CardDescription>
					Start each conversation from the agent with a friendly introduction.
					The introduction is <b>Markdown</b> syntax supported.
				</CardDescription>

				<ControlledAutosizeTextarea
					control={form.control}
					autoComplete="off"
					name="introductionMessage"
					maxHeight={300}
					placeholder="Give the agent a friendly introduction message."
				/>

				<p className="flex items-end justify-between pt-2 font-normal">
					Starter Messages
				</p>

				<small className="text-muted-foreground">
					Provide the user a list of suggestions to start a conversation with.
				</small>

				{starterMessages?.map((message, index) => (
					<div key={`starter-message-${index}`} className="flex gap-2">
						<ControlledInput
							control={form.control}
							name={`starterMessages.${index}`}
							classNames={{ wrapper: "flex-auto bg-background" }}
						/>
						<Button
							size="icon"
							variant="ghost"
							onClick={() =>
								form.setValue(
									"starterMessages",
									starterMessages.filter((_, i) => i !== index)
								)
							}
						>
							<TrashIcon />
						</Button>
					</div>
				))}

				<div className="flex w-full justify-end">
					<Button
						variant="ghost"
						className="self-end"
						startContent={<PlusIcon />}
						onClick={() =>
							form.setValue("starterMessages", [
								...(form.getValues("starterMessages") ?? []),
								"",
							])
						}
					>
						Add Message
					</Button>
				</div>
			</form>
		</Form>
	);
}
