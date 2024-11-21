import { useEffect } from "react";
import useSWR from "swr";

import { WorkflowService } from "~/lib/service/api/workflowService";

import { TypographyH4 } from "~/components/Typography";
import {
    ControlledCustomInput,
    ControlledInput,
} from "~/components/form/controlledInputs";
import { Button } from "~/components/ui/button";
import { FormItem, FormLabel } from "~/components/ui/form";
import { MultiSelect } from "~/components/ui/multi-select";
import {
    Select,
    SelectContent,
    SelectEmptyItem,
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
    const { form, handleSubmit, isLoading, isEdit, hasToken, hasSecret } =
        useWebhookFormContext();

    const getWorkflows = useSWR(WorkflowService.getWorkflows.key(), () =>
        WorkflowService.getWorkflows()
    );

    const workflows = getWorkflows.data;

    // note(ryanhopperlowe): this will change depending on webhook type
    const validationHeader = form.watch("validationHeader");
    useEffect(() => {
        if (!validationHeader) {
            form.setValue("validationHeader", "X-Hub-Signature-256");
        }
    }, [form, validationHeader]);

    return (
        <form onSubmit={handleSubmit} className="space-y-8 p-8">
            <TypographyH4>
                {isEdit ? "Edit Webhook" : "Create Webhook"}
            </TypographyH4>

            <ControlledInput control={form.control} name="name" label="Name" />

            <ControlledInput
                control={form.control}
                name="description"
                label="Description"
            />

            <FormItem>
                <FormLabel>Type</FormLabel>
                <Select value="Github" disabled>
                    <SelectTrigger>
                        <SelectValue />
                    </SelectTrigger>

                    <SelectContent>
                        <SelectItem value="Github">Github</SelectItem>
                    </SelectContent>
                </Select>
            </FormItem>

            {/* Extract to custom github component */}

            <ControlledCustomInput
                control={form.control}
                name="workflow"
                label="Workflow"
                description="The workflow that will be triggered when the webhook is called."
            >
                {({ field: { ref: _, ...field }, className }) => (
                    <Select {...field} onValueChange={field.onChange}>
                        <SelectTrigger className={className}>
                            <SelectValue placeholder="Select a workflow" />
                        </SelectTrigger>

                        <SelectContent>{getWorkflowOptions()}</SelectContent>
                    </Select>
                )}
            </ControlledCustomInput>

            <ControlledInput
                control={form.control}
                name="secret"
                label="Secret"
                description="This secret should match the secret you provide to GitHub."
                placeholder={hasSecret ? "(unchanged)" : ""}
            />

            <ControlledInput
                control={form.control}
                name="token"
                label="Token (optional)"
                description="Optionally provide a token to add an extra layer of security."
                placeholder={hasToken ? "(unchanged)" : ""}
            />

            <ControlledCustomInput
                control={form.control}
                name="headers"
                label="Headers"
            >
                {({ field }) => (
                    <MultiSelect
                        {...field}
                        options={GithubHeaderOptions}
                        value={field.value.map((v) => ({ label: v, value: v }))}
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
    );

    function getWorkflowOptions() {
        if (getWorkflows.isLoading)
            return (
                <SelectEmptyItem disabled>Loading workflows...</SelectEmptyItem>
            );

        if (!workflows?.length)
            return (
                <SelectEmptyItem disabled>No workflows found</SelectEmptyItem>
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
