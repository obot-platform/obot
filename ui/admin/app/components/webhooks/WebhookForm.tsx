import useSWR from "swr";

import { WorkflowService } from "~/lib/service/api/workflowService";
import { cn, getAliasFrom } from "~/lib/utils";

import { TypographyH2 } from "~/components/Typography";
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

type WebhookFormProps = WebhookFormContextProps;

export function WebhookForm(props: WebhookFormProps) {
    return (
        <WebhookFormContextProvider {...props}>
            <WebhookFormContent />
        </WebhookFormContextProvider>
    );
}

export function WebhookFormContent() {
    const { form, handleSubmit, isLoading, isEdit, hasSecret } =
        useWebhookFormContext();

    const { watch, control } = form;

    const getWorkflows = useSWR(WorkflowService.getWorkflows.key(), () =>
        WorkflowService.getWorkflows()
    );

    const workflows = getWorkflows.data;

    const removeSecret = watch("removeSecret");

    return (
        <ScrollArea className="h-full">
            <form
                className="space-y-8 p-8 max-w-3xl mx-auto"
                onSubmit={handleSubmit}
            >
                <TypographyH2>
                    {isEdit ? "Edit Webhook" : "Create Webhook"}
                </TypographyH2>

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
                    name="workflow"
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

                            <SelectContent>
                                {getWorkflowOptions()}
                            </SelectContent>
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
                            removeSecret
                                ? "(removed)"
                                : hasSecret
                                  ? "(unchanged)"
                                  : ""
                        }
                        disabled={removeSecret}
                    />

                    {hasSecret && (
                        <FormField
                            control={control}
                            name="removeSecret"
                            render={({
                                field: { value, onChange, ...field },
                            }) => (
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
                            onChange={(value) =>
                                field.onChange(value.map((v) => v.value))
                            }
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
        const workflow = watch("workflow");

        if (getWorkflows.isLoading)
            return (
                <SelectItem value={workflow || "loading"} disabled>
                    Loading workflows...
                </SelectItem>
            );

        if (!workflows?.length)
            return (
                <SelectItem value={workflow || "empty"} disabled>
                    No workflows found
                </SelectItem>
            );

        return workflows.map((workflow) => (
            <SelectItem key={workflow.id} value={workflow.id}>
                {workflow.name}
            </SelectItem>
        ));
    }
}
