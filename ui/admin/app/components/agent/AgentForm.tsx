import { zodResolver } from "@hookform/resolvers/zod";
import { BrainIcon } from "lucide-react";
import { useEffect, useMemo } from "react";
import { useForm } from "react-hook-form";
import useSWR from "swr";
import { z } from "zod";

import { ModelUsage } from "~/lib/model/models";
import { ModelApiService } from "~/lib/service/api/modelApiService";

import { TypographyH4 } from "~/components/Typography";
import {
    ControlledAutosizeTextarea,
    ControlledCustomInput,
    ControlledInput,
} from "~/components/form/controlledInputs";
import { Form } from "~/components/ui/form";
import {
    Select,
    SelectContent,
    SelectEmptyItem,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from "~/components/ui/select";

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
    const getModels = useSWR(
        ModelApiService.getModels.key(),
        ModelApiService.getModels
    );

    const models = useMemo(() => {
        if (!getModels.data) return [];

        return getModels.data.filter(
            (m) => !m.usage || m.usage === ModelUsage.LLM
        );
    }, [getModels.data]);

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

                <TypographyH4 className="flex items-center gap-2">
                    <BrainIcon className="w-5 h-5" />
                    Instructions
                </TypographyH4>

                <ControlledAutosizeTextarea
                    control={form.control}
                    autoComplete="off"
                    name="prompt"
                    maxHeight={300}
                    placeholder="Give the agent instructions on how to behave and respond to input."
                />

                <ControlledCustomInput
                    label="Model"
                    control={form.control}
                    name="model"
                >
                    {({ field: { ref: _, ...field } }) => (
                        <Select {...field} onValueChange={field.onChange}>
                            <SelectTrigger>
                                <SelectValue placeholder="Use System Default" />
                            </SelectTrigger>

                            <SelectContent>
                                <SelectEmptyItem>
                                    Use System Default
                                </SelectEmptyItem>

                                {models.map((m) => (
                                    <SelectItem key={m.id} value={m.id}>
                                        {m.name || m.id}{" "}
                                        <span className="text-muted-foreground">
                                            ({m.modelProvider})
                                        </span>
                                    </SelectItem>
                                ))}
                            </SelectContent>
                        </Select>
                    )}
                </ControlledCustomInput>
            </form>
        </Form>
    );
}
