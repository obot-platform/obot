import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect } from "react";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { Agent } from "~/lib/model/agents";

import {
    ControlledInput,
    ControlledTextarea,
} from "~/components/form/controlledInputs";
import { Form } from "~/components/ui/form";

const formSchema = z.object({
    name: z.string().min(1, {
        message: "Name is required.",
    }),
    description: z.string().optional(),
    prompt: z.string().optional(),
});

export type AgentInfoFormValues = z.infer<typeof formSchema>;

type AgentFormProps = {
    agent: Agent;
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
            <form onSubmit={handleSubmit} className="space-y-8">
                <ControlledInput
                    control={form.control}
                    name="name"
                    label="Name"
                    placeholder="Give the agent a name."
                />

                <ControlledTextarea
                    control={form.control}
                    rows={4}
                    name="description"
                    label="Description"
                    placeholder="Describe the agent."
                />

                <ControlledTextarea
                    control={form.control}
                    rows={4}
                    name="prompt"
                    label="Prompt"
                    placeholder="What should the agent do?"
                />
            </form>
        </Form>
    );
}
