import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect } from "react";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { Step, Workflow } from "~/lib/model/workflows";
import { noop } from "~/lib/utils";

import { Form, FormField, FormItem, FormMessage } from "~/components/ui/form";
import { AddStepButton } from "~/components/workflow/steps/AddStep";
import { renderStep } from "~/components/workflow/steps/StepRenderer";

const formSchema = z.object({
    steps: z.array(z.custom<Step>()),
});

export type StepsFormValues = z.infer<typeof formSchema>;

export function StepsForm({
    workflow,
    onSubmit,
    onChange,
}: {
    workflow: Workflow;
    onSubmit?: (values: StepsFormValues) => void;
    onChange?: (values: StepsFormValues) => void;
}) {
    const form = useForm<StepsFormValues>({
        resolver: zodResolver(formSchema),
        defaultValues: { steps: workflow.steps || [] },
    });

    const handleSubmit = form.handleSubmit(onSubmit || noop);

    const stepValues = form.watch("steps");

    useEffect(() => {
        form.reset({ steps: workflow.steps || [] });
    }, [workflow, form]);

    useEffect(() => {
        return form.watch((values) => {
            const { data, success } = formSchema.safeParse(values);
            if (!success) return;
            onChange?.(data);
        }).unsubscribe;
    }, [stepValues, form.formState, onChange, form]);

    return (
        <Form {...form}>
            <form onSubmit={handleSubmit}>
                <FormField
                    control={form.control}
                    name="steps"
                    render={({ field }) => (
                        <FormItem>
                            <div className="space-y-4 mb-2">
                                {field.value.map((step, index) =>
                                    renderStep(
                                        step,
                                        (updatedStep) => {
                                            const newSteps = [...field.value];
                                            newSteps[index] = updatedStep;
                                            field.onChange(newSteps);
                                        },
                                        () => {
                                            const newSteps = field.value.filter(
                                                (_, i) => i !== index
                                            );
                                            field.onChange(newSteps);
                                        }
                                    )
                                )}
                            </div>
                            <AddStepButton
                                onAddStep={(newStep) => {
                                    field.onChange([...field.value, newStep]);
                                }}
                            />
                            <FormMessage />
                        </FormItem>
                    )}
                />
            </form>
        </Form>
    );
}
