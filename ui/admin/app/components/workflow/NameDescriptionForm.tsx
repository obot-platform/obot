import { zodResolver } from "@hookform/resolvers/zod";
import { PlusIcon, TrashIcon } from "lucide-react";
import { useEffect } from "react";
import { useFieldArray, useForm } from "react-hook-form";
import { z } from "zod";

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

type Item = {
    name: string;
    description: string;
};

export function NameDescriptionForm({
    defaultValues,
    onChange,
    addLabel = "Add",
}: {
    defaultValues: Item[];
    onChange: (values: Item[]) => void;
    addLabel?: string;
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
                    variant="ghost"
                    className="self-end"
                    startContent={<PlusIcon />}
                    onClick={() =>
                        paramFields.append({ name: "", description: "" })
                    }
                >
                    {addLabel}
                </Button>
            </div>
        </Form>
    );
}
