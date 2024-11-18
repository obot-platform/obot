import { zodResolver } from "@hookform/resolvers/zod";
import { PlusIcon, TrashIcon } from "lucide-react";
import { useEffect, useMemo } from "react";
import { useFieldArray, useForm } from "react-hook-form";
import { z } from "zod";

import { Workflow } from "~/lib/model/workflows";

import { ControlledInput } from "~/components/form/controlledInputs";
import { Button } from "~/components/ui/button";
import { Form } from "~/components/ui/form";

const formSchema = z.object({
    params: z.array(
        z.object({
            name: z.string(),
            description: z.string(),
        })
    ),
});

export type ParamFormValues = z.infer<typeof formSchema>;

type ParamValues = Workflow["params"];

const convertFrom = (params: ParamValues) => {
    const converted = Object.entries(params || {}).map(
        ([name, description]) => ({
            name,
            description,
        })
    );

    return {
        params: converted.length ? converted : [{ name: "", description: "" }],
    };
};

const convertTo = (params: ParamFormValues["params"]) => {
    if (!params?.length) return undefined;

    return params.reduce((acc, param) => {
        if (!param.name) return acc;

        acc[param.name] = param.description;
        return acc;
    }, {} as NonNullable<ParamValues>);
};

export function ParamsForm({
    workflow,
    onChange,
}: {
    workflow: Workflow;
    onChange?: (values: { params?: ParamValues }) => void;
}) {
    const defaultValues = useMemo(
        () => convertFrom(workflow.params),
        [workflow.params]
    );

    const form = useForm<ParamFormValues>({
        resolver: zodResolver(formSchema),
        defaultValues,
    });

    const paramFields = useFieldArray({
        control: form.control,
        name: "params",
    });

    useEffect(() => {
        const subscription = form.watch((value, { name, type }) => {
            if (name === "params" || type === "change") {
                const { data, success } = formSchema.safeParse(value);

                if (success) {
                    onChange?.({ params: convertTo(data.params) });
                }
            }
        });
        return () => subscription.unsubscribe();
    }, [form, onChange]);

    return (
        <Form {...form}>
            <div className="flex flex-col gap-4">
                {paramFields.fields.map((field, i) => (
                    <div
                        className="flex gap-2 p-2 bg-secondary rounded-md"
                        key={field.id}
                    >
                        <ControlledInput
                            control={form.control}
                            name={`params.${i}.name`}
                            placeholder="Name"
                            classNames={{ wrapper: "flex-auto bg-background" }}
                        />

                        <ControlledInput
                            control={form.control}
                            name={`params.${i}.description`}
                            placeholder="Description"
                            classNames={{ wrapper: "flex-auto bg-background" }}
                        />

                        <Button
                            variant="ghost"
                            size="icon"
                            onClick={() => paramFields.remove(i)}
                        >
                            <TrashIcon />
                        </Button>
                    </div>
                ))}

                <Button
                    variant="secondary"
                    className="self-end"
                    startContent={<PlusIcon />}
                    onClick={() =>
                        paramFields.append({ name: "", description: "" })
                    }
                >
                    Add Parameter
                </Button>
            </div>
        </Form>
    );
}
