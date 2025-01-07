import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect } from "react";
import { useForm } from "react-hook-form";
import { useNavigate } from "react-router";
import { $path } from "safe-routes";
import { toast } from "sonner";
import useSWR, { mutate } from "swr";
import { z } from "zod";

import { EmailReceiver } from "~/lib/model/email-receivers";
import { EmailReceiverApiService } from "~/lib/service/api/emailReceiverApiService";
import { WorkflowService } from "~/lib/service/api/workflowService";

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
    workflow: z.string().min(1, "Workflow is required"),
    allowedSenders: z.array(z.string()),
});

export type EmailRecieverFormValues = z.infer<typeof formSchema>;

type EmailRecieverFormProps = {
    emailReceiver?: EmailReceiver;
};

export function EmailReceiverForm({ emailReceiver }: EmailRecieverFormProps) {
    const navigate = useNavigate();
    const getWorkflows = useSWR(WorkflowService.getWorkflows.key(), () =>
        WorkflowService.getWorkflows()
    );

    const handleSubmitSuccess = () => {
        if (emailReceiver) {
            mutate(
                EmailReceiverApiService.getEmailReceiverById(emailReceiver.id)
            );
        }
        mutate(EmailReceiverApiService.getEmailReceivers.key());
        navigate($path("/workflow-triggers"));
    };

    const form = useForm<EmailRecieverFormValues>({
        resolver: zodResolver(formSchema),
        mode: "onChange",
        defaultValues: {
            name: emailReceiver?.name || "",
            description: emailReceiver?.description || "",
            alias: emailReceiver?.alias || "",
            workflow: emailReceiver?.workflow || "",
            allowedSenders: emailReceiver?.allowedSenders || [],
        },
    });

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
            form.reset(emailReceiver);
        }
    }, [emailReceiver, form]);

    const handleSubmit = form.handleSubmit((values: EmailRecieverFormValues) =>
        emailReceiver?.id
            ? updateEmailReceiver.execute(emailReceiver.id, values)
            : createEmailReceiver.execute(values)
    );

    const workflows = getWorkflows.data;
    const isEdit = !!emailReceiver?.id;
    const loading =
        createEmailReceiver.isLoading || updateEmailReceiver.isLoading;

    return (
        <ScrollArea className="h-full">
            <Form {...form}>
                <form
                    className="space-y-8 p-8 max-w-3xl mx-auto"
                    onSubmit={handleSubmit}
                >
                    <h2>{isEdit ? "Edit" : "Create"} Email Receiver</h2>

                    <ControlledInput
                        control={form.control}
                        name="name"
                        label="Name"
                    />

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
                        name="workflow"
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

                                <SelectContent>
                                    {getWorkflowOptions()}
                                </SelectContent>
                            </Select>
                        )}
                    </ControlledCustomInput>

                    <Button
                        className="w-full"
                        type="submit"
                        disabled={loading}
                        loading={loading}
                    >
                        {isEdit ? "Update" : "Create"} Email Receiver
                    </Button>
                </form>
            </Form>
        </ScrollArea>
    );

    function getWorkflowOptions() {
        const workflow = form.watch("workflow");

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
