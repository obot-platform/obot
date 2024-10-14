import { zodResolver } from "@hookform/resolvers/zod";
import { Plus, TrashIcon } from "lucide-react";
import { useEffect, useState } from "react";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { Workflow } from "~/lib/model/workflows";
import { noop } from "~/lib/utils";

import { Button } from "~/components/ui/button";
import { Form, FormField, FormItem, FormMessage } from "~/components/ui/form";
import { Input } from "~/components/ui/input";

const formSchema = z.object({
    params: z.record(z.string(), z.string()).optional(),
});

export type ParamFormValues = z.infer<typeof formSchema>;

export function ParamsForm({
    workflow,
    onSubmit,
    onChange,
}: {
    workflow: Workflow;
    onSubmit?: (values: ParamFormValues) => void;
    onChange?: (values: ParamFormValues) => void;
}) {
    const form = useForm<ParamFormValues>({
        resolver: zodResolver(formSchema),
        defaultValues: { params: workflow.params || {} },
    });

    const handleSubmit = form.handleSubmit(onSubmit || noop);

    const [newParamKey, setNewParamKey] = useState("");
    const [newParamValue, setNewParamValue] = useState("");

    useEffect(() => {
        const subscription = form.watch((value, { name, type }) => {
            if (name === "params" || type === "change") {
                const { data, success } = formSchema.safeParse(value);
                if (success) {
                    onChange?.(data);
                }
            }
        });
        return () => subscription.unsubscribe();
    }, [form, onChange]);

    return (
        <Form {...form}>
            <form onSubmit={handleSubmit}>
                <FormField
                    control={form.control}
                    name="params"
                    render={({ field }) => (
                        <FormItem>
                            <div className="flex space-x-2">
                                <Input
                                    placeholder="Name"
                                    value={newParamKey}
                                    onChange={(e) =>
                                        setNewParamKey(e.target.value)
                                    }
                                    className="flex-grow"
                                />
                                <Input
                                    placeholder="Description"
                                    value={newParamValue}
                                    onChange={(e) =>
                                        setNewParamValue(e.target.value)
                                    }
                                    className="flex-grow"
                                />
                                <Button
                                    type="button"
                                    size="icon"
                                    className="flex-shrink-0"
                                    variant="secondary"
                                    onClick={() => {
                                        if (newParamKey && newParamValue) {
                                            const updatedParams = {
                                                ...field.value,
                                            };
                                            updatedParams[newParamKey] =
                                                newParamValue;
                                            field.onChange(updatedParams);
                                            setNewParamKey("");
                                            setNewParamValue("");
                                        }
                                    }}
                                >
                                    <Plus className="w-4 h-4" />
                                </Button>
                            </div>

                            <div className="mt-2 w-full">
                                {Object.entries(field.value || {}).map(
                                    ([key, value], index) => (
                                        <div
                                            key={index}
                                            className="flex items-center space-x-2 justify-between mt-2"
                                        >
                                            <Input
                                                disabled
                                                className="cursor-not-allowed"
                                                value={key}
                                            />
                                            <Input
                                                value={value}
                                                onChange={(e) => {
                                                    const updatedParams = {
                                                        ...field.value,
                                                    };
                                                    updatedParams[key] =
                                                        e.target.value;
                                                    field.onChange(
                                                        updatedParams
                                                    );
                                                }}
                                                className="flex-grow"
                                            />
                                            <Button
                                                type="button"
                                                variant="destructive"
                                                size="icon"
                                                className="flex-shrink-0"
                                                onClick={() => {
                                                    const updatedParams = {
                                                        ...field.value,
                                                    };
                                                    delete updatedParams[key];
                                                    field.onChange(
                                                        updatedParams
                                                    );
                                                }}
                                            >
                                                <TrashIcon className="w-4 h-4" />
                                            </Button>
                                        </div>
                                    )
                                )}
                            </div>
                            <FormMessage />
                        </FormItem>
                    )}
                />
            </form>
        </Form>
    );
}
