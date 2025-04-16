import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect } from "react";
import { useForm } from "react-hook-form";
import { toast } from "sonner";
import useSWR, { mutate } from "swr";
import { z } from "zod";

import { EmailReceiver } from "~/lib/model/email-receivers";
import { EmailReceiverApiService } from "~/lib/service/api/emailReceiverApiService";
import { TaskService } from "~/lib/service/api/taskService";

import {
	ControlledCustomInput,
	ControlledInput,
} from "~/components/form/controlledInputs";
import { Button } from "~/components/ui/button";
import { Form } from "~/components/ui/form";
import { ScrollArea } from "~/components/ui/scroll-area";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "~/components/ui/select";
import { useAsync } from "~/hooks/useAsync";

const formSchema = z.object({
	name: z.string().min(1, "Name is required"),
	description: z.string(),
	alias: z.string(),
	workflowName: z.string().min(1, "WorkflowName is required"),
	allowedSenders: z.array(z.string()).optional(),
});

export type EmailRecieverFormValues = z.infer<typeof formSchema>;

type EmailRecieverFormProps = {
	emailReceiver?: Partial<EmailReceiver>;
	onContinue?: () => void;
	hideTitle?: boolean;
};

export function EmailReceiverForm({
	emailReceiver,
	onContinue,
	hideTitle,
}: EmailRecieverFormProps) {
	const getTasks = useSWR(...TaskService.getTasks.swr({}), {
		fallbackData: [],
	});

	const handleSubmitSuccess = () => {
		if (emailReceiver?.id) {
			mutate(EmailReceiverApiService.getEmailReceiverById(emailReceiver.id));
		}

		EmailReceiverApiService.getEmailReceivers.revalidate();
		onContinue?.();
	};

	const form = useForm<EmailRecieverFormValues>({
		resolver: zodResolver(formSchema),
		mode: "onChange",
		defaultValues: {
			name: emailReceiver?.name || "",
			description: emailReceiver?.description || "",
			alias: emailReceiver?.alias || "",
			workflowName: emailReceiver?.workflowName || "",
			allowedSenders: emailReceiver?.allowedSenders || [],
		},
	});

	const { handleSubmit, reset } = form;

	const createEmailReceiver = useAsync(
		EmailReceiverApiService.createEmailReceiver,
		{
			onSuccess: handleSubmitSuccess,
			onError: () => {
				toast.error("Failed to create email receiver.");
			},
		}
	);

	const updateEmailReceiver = useAsync(
		EmailReceiverApiService.updateEmailReceiver,
		{
			onSuccess: handleSubmitSuccess,
			onError: () => {
				toast.error("Failed to update email receiver.");
			},
		}
	);

	useEffect(() => {
		if (emailReceiver) {
			reset(emailReceiver);
		}
	}, [emailReceiver, reset]);

	const onSubmit = handleSubmit((values: EmailRecieverFormValues) =>
		emailReceiver?.id
			? updateEmailReceiver.execute(emailReceiver.id, values)
			: createEmailReceiver.execute(values)
	);

	const workflows = getTasks.data;
	const isEdit = !!emailReceiver?.id;
	const loading =
		createEmailReceiver.isLoading || updateEmailReceiver.isLoading;

	return (
		<ScrollArea className="h-full">
			<Form {...form}>
				<form className="mx-auto max-w-3xl space-y-8 p-8" onSubmit={onSubmit}>
					{!hideTitle && <h2>{isEdit ? "Edit" : "Create"} Email Trigger</h2>}

					<ControlledInput control={form.control} name="name" label="Name" />

					<ControlledInput
						control={form.control}
						name="description"
						label="Description (Optional)"
					/>

					<ControlledInput
						control={form.control}
						name="alias"
						label="Alias (Optional)"
					/>

					<ControlledCustomInput
						control={form.control}
						name="workflowName"
						label="Workflow"
						description="The workflow that will be called when an email is received."
					>
						{({ field: { ref: _, ...field }, className }) => (
							<Select
								defaultValue={field.value}
								onValueChange={field.onChange}
								key={field.value}
							>
								<SelectTrigger className={className}>
									<SelectValue placeholder="Select a workflow" />
								</SelectTrigger>

								<SelectContent>{getWorkflowOptions()}</SelectContent>
							</Select>
						)}
					</ControlledCustomInput>

					<Button
						className="w-full"
						type="submit"
						disabled={loading}
						loading={loading}
					>
						{isEdit ? "Update" : "Create"} Email Trigger
					</Button>
				</form>
			</Form>
		</ScrollArea>
	);

	function getWorkflowOptions() {
		const workflow = form.watch("workflowName");

		if (getTasks.isLoading)
			return (
				<SelectItem value={workflow || "loading"} disabled>
					Loading workflows...
				</SelectItem>
			);

		if (!workflows?.length)
			return (
				<SelectItem value={workflow || "empty"} disabled>
					No tasks found
				</SelectItem>
			);

		return workflows.map((workflow) => (
			<SelectItem key={workflow.id} value={workflow.id}>
				{workflow.name}
			</SelectItem>
		));
	}
}
