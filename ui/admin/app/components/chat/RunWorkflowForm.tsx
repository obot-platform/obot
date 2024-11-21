import { useMemo } from "react";
import { useForm } from "react-hook-form";

import { ControlledInput } from "~/components/form/controlledInputs";
import { Button } from "~/components/ui/button";
import { Form } from "~/components/ui/form";

type RunWorkflowFormProps = {
    params: Record<string, string>;
    onSubmit: (params: Record<string, string>) => void;
};

export function RunWorkflowForm({ params, onSubmit }: RunWorkflowFormProps) {
    const defaultValues = useMemo(() => {
        return Object.keys(params).reduce(
            (acc, key) => {
                acc[key] = "";
                return acc;
            },
            {} as Record<string, string>
        );
    }, [params]);

    const form = useForm({ defaultValues });
    const handleSubmit = form.handleSubmit(onSubmit);

    return (
        <Form {...form}>
            <form onSubmit={handleSubmit} className="flex flex-col gap-2">
                {Object.entries(params).map(([name, description]) => (
                    <ControlledInput
                        key={name}
                        control={form.control}
                        name={name}
                        label={name}
                        description={description}
                    />
                ))}

                <Button type="submit">Run Workflow</Button>
            </form>
        </Form>
    );
}
