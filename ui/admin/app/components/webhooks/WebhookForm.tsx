import useSWR from "swr";

import { TaskService } from "~/lib/service/api/taskService";
import { cn, getAliasFrom } from "~/lib/utils";

import {
	ControlledCustomInput,
	ControlledInput,
} from "~/components/form/controlledInputs";
import { Button } from "~/components/ui/button";
import { Checkbox } from "~/components/ui/checkbox";
import {
	FormControl,
	FormField,
	FormItem,
	FormLabel,
} from "~/components/ui/form";
import { MultiSelect } from "~/components/ui/multi-select";
import { ScrollArea } from "~/components/ui/scroll-area";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "~/components/ui/select";
import {
	WebhookFormContextProps,
	WebhookFormContextProvider,
	useWebhookFormContext,
} from "~/components/webhooks/WebhookFormContext";

type WebhookFormContentProps = {
	hideTitle?: boolean;
};

type WebhookFormProps = WebhookFormContextProps & WebhookFormContentProps;

export function WebhookForm({ hideTitle, ...props }: WebhookFormProps) {
	return (
		<WebhookFormContextProvider {...props}>
			<WebhookFormContent hideTitle={hideTitle} />
		</WebhookFormContextProvider>
	);
}

export function WebhookFormContent({ hideTitle }: WebhookFormContentProps) {
	const { form, handleSubmit, isLoading, isEdit, hasSecret } =
		useWebhookFormContext();

	const { watch, control } = form;

	const getTasks = useSWR(...TaskService.getTasks.swr({}));

	const tasks = getTasks.data;

	const removeSecret = watch("removeSecret");

	return (
		<ScrollArea className="h-full">
			<form className="mx-auto max-w-3xl space-y-8 p-8" onSubmit={handleSubmit}>
				{!hideTitle && <h2>{isEdit ? "Edit Webhook" : "Create Webhook"}</h2>}

				<ControlledInput control={control} name="name" label="Name" />

				<ControlledInput
					control={form.control}
					name="alias"
					label="Alias (Optional)"
					description="This will be used to construct the webhook URL."
					onChangeConversion={getAliasFrom}
				/>

				<ControlledInput
					control={control}
					name="description"
					label="Description (Optional)"
				/>

				<ControlledCustomInput
					control={control}
					name="workflowName"
					label="Workflow"
					description="The workflow that will be triggered when the webhook is called."
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

				<div className="space-y-2">
					<ControlledInput
						control={control}
						name="secret"
						label="Payload Signature Secret (Optional)"
						description="This should match the secret you provide to the webhook provider."
						placeholder={
							removeSecret ? "(removed)" : hasSecret ? "(unchanged)" : ""
						}
						disabled={removeSecret}
					/>

					{hasSecret && (
						<FormField
							control={control}
							name="removeSecret"
							render={({ field: { value, onChange, ...field } }) => (
								<FormItem className="flex items-center gap-2 space-y-0">
									<FormControl>
										<Checkbox
											{...field}
											checked={value}
											onCheckedChange={onChange}
										/>
									</FormControl>

									<FormLabel>No Secret</FormLabel>
								</FormItem>
							)}
						/>
					)}
				</div>

				<ControlledInput
					classNames={{ wrapper: cn({ hidden: removeSecret }) }}
					control={control}
					name="validationHeader"
					label="Payload Signature Header (Optional)"
					description="The webhook receiver will calculate an HMAC digest of the payload using the supplied secret and compare it to the value sent in this header."
				/>

				<ControlledCustomInput
					control={control}
					name="headers"
					label="Headers (Optional)"
					description={`Add "*" to include all headers.`}
				>
					{({ field }) => (
						<MultiSelect
							{...field}
							options={[]}
							value={field.value.map((v) => ({
								label: v,
								value: v,
							}))}
							creatable
							onChange={(value) => field.onChange(value.map((v) => v.value))}
							side="top"
						/>
					)}
				</ControlledCustomInput>

				<Button
					className="w-full"
					type="submit"
					disabled={isLoading}
					loading={isLoading}
				>
					{isEdit ? "Update Webhook" : "Create Webhook"}
				</Button>
			</form>
		</ScrollArea>
	);

	function getWorkflowOptions() {
		const workflow = watch("workflowName");

		if (getTasks.isLoading)
			return (
				<SelectItem value={workflow || "loading"} disabled>
					Loading workflows...
				</SelectItem>
			);

		if (!tasks?.length)
			return (
				<SelectItem value={workflow || "empty"} disabled>
					No workflows found
				</SelectItem>
			);

		return tasks.map((task) => (
			<SelectItem key={task.id} value={task.id}>
				{task.name}
			</SelectItem>
		));
	}
}
