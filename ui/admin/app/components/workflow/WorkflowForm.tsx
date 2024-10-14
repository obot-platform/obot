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
import { Input } from "~/components/ui/input";

import { Textarea } from "../ui/textarea";

const formSchema = z.object({
    name: z.string().min(1, {
        message: "Name is required.",
    }),
    description: z.string().optional(),
    prompt: z.string().optional(),
});

export type WorkflowInfoFormValues = z.infer<typeof formSchema>;

type WorkflowFormProps = {
    workflow: Workflow;
    onSubmit?: (values: WorkflowInfoFormValues) => void;
    onChange?: (values: WorkflowInfoFormValues) => void;
};

export function WorkflowForm({
    workflow,
    onSubmit,
    onChange,
}: WorkflowFormProps) {
    const form = useForm<WorkflowInfoFormValues>({
        resolver: zodResolver(formSchema),
        defaultValues: {
            name: workflow.name || "",
            description: workflow.description || "",
            prompt: workflow.prompt || "",
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

    const handleSubmit = form.handleSubmit((values: WorkflowInfoFormValues) =>
        onSubmit?.({ ...workflow, ...values })
    );

    return (
        <Form {...form}>
            <form onSubmit={handleSubmit} className="space-y-8">
                <FormField
                    control={form.control}
                    name="name"
                    render={({ field }) => (
                        <FormItem>
                            <FormLabel>Name</FormLabel>

                            <FormMessage />
                            <FormControl>
                                <Input
                                    placeholder="Give the workflow a name."
                                    {...field}
                                />
                            </FormControl>
                        </FormItem>
                    )}
                />
                <FormField
                    control={form.control}
                    name="description"
                    render={({ field }) => (
                        <FormItem>
                            <FormLabel>Description</FormLabel>

                            <FormControl>
                                <Textarea
                                    rows={2}
                                    placeholder="Describe the workflow."
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
