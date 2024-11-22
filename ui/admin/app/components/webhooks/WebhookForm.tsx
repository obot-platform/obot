import useSWR from "swr";

import { WorkflowService } from "~/lib/service/api/workflowService";

import { TypographyH3, TypographyH4 } from "~/components/Typography";
import {
    ControlledCustomInput,
    ControlledInput,
} from "~/components/form/controlledInputs";
import { Button } from "~/components/ui/button";
import { Checkbox } from "~/components/ui/checkbox";
import { FormControl, FormItem, FormLabel } from "~/components/ui/form";
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

    const { setValue, watch, control } = form;

    const getWorkflows = useSWR(WorkflowService.getWorkflows.key(), () =>
        WorkflowService.getWorkflows()
    );

    const workflows = getWorkflows.data;

    const validationHeader = watch("validationHeader");
    const secret = watch("secret");

    const removeSecret = () => {
        setValue("secret", "");
        setValue("validationHeader", "");
    };

    const addSecret = () => setValue("validationHeader", "X-Hub-Signature-256");

    const secretIsRemoved = hasSecret && !validationHeader && !secret;

    return (
        <ScrollArea className="h-full">
            <form
                className="space-y-8 p-8 max-w-3xl mx-auto"
                onSubmit={handleSubmit}
            >
                <TypographyH3>
                    {isEdit ? "Edit Webhook" : "Create Webhook"}
                </TypographyH3>

                <ControlledInput control={control} name="name" label="Name" />

                {/* todo(ryanhopperlowe): Re-enable alias after go ahead is given */}
                {/* <ControlledInput
                    control={form.control}
                    name="alias"
                    label="Alias (Optional)"
                    description={
                        alias
                            ? `Aliased URL: ${ApiRoutes.webhooks.invoke(alias).url}`
                            : "An alias is a short name for the webhook to make it easier to identify."
                    }
                    onChangeConversion={getAliasFrom}
                /> */}

                <ControlledInput
                    control={control}
                    name="description"
                    label="Description (Optional)"
                />

                <FormItem>
                    <FormLabel>Type</FormLabel>
                    <Select value="GitHub" disabled>
                        <SelectTrigger>
                            <SelectValue />
                        </SelectTrigger>

                        <SelectContent>
                            <SelectItem value="GitHub">GitHub</SelectItem>
                        </SelectContent>
                    </Select>
                </FormItem>

                {/* Extract to custom github component */}

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
                        label="Secret (Optional)"
                        description="This secret should match the secret you provide to GitHub."
                        placeholder={
                            secretIsRemoved
                                ? "(removed)"
                                : hasSecret
                                  ? "(unchanged)"
                                  : ""
                        }
                        disabled={secretIsRemoved}
                        onChange={(e) => {
                            if (!hasSecret && e.target.value) addSecret();
                        }}
                    />

                    {hasSecret && (
                        <FormItem className="flex items-center gap-2 space-y-0">
                            <FormControl>
                                <Checkbox
                                    checked={secretIsRemoved}
                                    onCheckedChange={(val) => {
                                        if (val) removeSecret();
                                        else addSecret();
                                    }}
                                />
                            </FormControl>

                            <FormLabel>No Secret</FormLabel>
                        </FormItem>
                    )}
                </div>

                <TypographyH4>Advanced</TypographyH4>

                <ControlledCustomInput
                    control={control}
                    name="headers"
                    label="Headers (Optional)"
                    description={`Add "*" to include all headers.`}
                >
                    {({ field }) => (
                        <MultiSelect
                            {...field}
                            options={GithubHeaderOptions}
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

const GithubHeaderOptions = [
    "X-GitHub-Hook-ID",
    "X-GitHub-Event",
    "X-GitHub-Delivery",
    "User-Agent",
    "X-GitHub-Hook-Installation-Target-Type",
    "X-GitHub-Hook-Installation-Target-ID",
].map((header) => ({
    label: header,
    value: header,
}));
