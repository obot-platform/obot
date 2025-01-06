import { zodResolver } from "@hookform/resolvers/zod";
import { PlusIcon, TrashIcon } from "lucide-react";
import { useEffect } from "react";
import { useFieldArray, useForm } from "react-hook-form";
import { z } from "zod";

import { cn } from "~/lib/utils";

import { ControlledInput } from "~/components/form/controlledInputs";
import { Button } from "~/components/ui/button";
import { Card } from "~/components/ui/card";
import { Form } from "~/components/ui/form";
import { InputProps } from "~/components/ui/input";

const formSchema = z.object({
    params: z.array(
        z.object({
            name: z.string(),
            description: z.string(),
        })
    ),
});

export type ParamFormValues = z.infer<typeof formSchema>;

type Item = {
    name: string;
    description: string;
};

export function NameDescriptionForm({
    defaultValues,
    onChange,
    addLabel = "Add",
    nameFieldProps,
    descriptionFieldProps,
    asCard = false,
}: {
    defaultValues: Item[];
    onChange: (values: Item[]) => void;
    addLabel?: string;
    nameFieldProps?: InputProps;
    descriptionFieldProps?: InputProps;
    asCard?: boolean;
}) {
    const form = useForm<ParamFormValues>({
        resolver: zodResolver(formSchema),
        defaultValues: { params: defaultValues },
    });

    const paramFields = useFieldArray({
        control: form.control,
        name: "params",
    });

    useEffect(() => {
        const subscription = form.watch((value) => {
            const { data, success } = formSchema.safeParse(value);

            if (success) {
                onChange?.(data.params);
            }
        });
        return () => subscription.unsubscribe();
    }, [form, onChange]);

    const Comp = asCard ? Card : "div";
    const isEmpty = paramFields.fields.length === 0;

    return (
        <Form {...form}>
            <Comp
                className={cn("flex flex-col gap-4 fade-in-50", {
                    "p-4": asCard,
                    hidden: isEmpty,
                })}
            >
                {paramFields.fields.map((field, i) => (
                    <div className="flex gap-2" key={field.id}>
                        <ControlledInput
                            placeholder="Name"
                            {...nameFieldProps}
                            control={form.control}
                            name={`params.${i}.name`}
                            classNames={{ wrapper: "flex-auto bg-background" }}
                        />

                        <ControlledInput
                            placeholder="Description"
                            {...descriptionFieldProps}
                            control={form.control}
                            name={`params.${i}.description`}
                            classNames={{ wrapper: "flex-auto bg-background" }}
                        />

                        <Button
                            variant="ghost"
                            size="icon"
                            type="button"
                            onClick={() => paramFields.remove(i)}
                        >
                            <TrashIcon />
                        </Button>
                    </div>
                ))}
            </Comp>

            <Button
                variant="ghost"
                className="self-end"
                startContent={<PlusIcon />}
                type="button"
                onClick={() =>
                    paramFields.append({ name: "", description: "" })
                }
            >
                {addLabel}
            </Button>
        </Form>
    );
}
