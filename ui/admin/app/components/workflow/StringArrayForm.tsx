import { zodResolver } from "@hookform/resolvers/zod";
import { Plus, TrashIcon } from "lucide-react";
import { useEffect, useState } from "react";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { noop } from "~/lib/utils";

import { Form, FormField, FormItem, FormMessage } from "~/components/ui/form";
import { Input } from "~/components/ui/input";

import { Button } from "../ui/button";

const formSchema = z.object({
    items: z.array(z.string()),
});

export type StringArrayFormValues = z.infer<typeof formSchema>;

export function StringArrayForm({
    initialItems = [],
    onSubmit,
    onChange,
    placeholder,
}: {
    initialItems?: string[];
    onSubmit?: (values: StringArrayFormValues) => void;
    onChange?: (values: StringArrayFormValues) => void;
    itemName: string;
    placeholder: string;
}) {
    const form = useForm<StringArrayFormValues>({
        resolver: zodResolver(formSchema),
        defaultValues: { items: initialItems },
    });

    const handleSubmit = form.handleSubmit(onSubmit || noop);

    const [newItem, setNewItem] = useState("");

    const itemValues = form.watch("items");

    useEffect(() => {
        return form.watch((values) => {
            const { data, success } = formSchema.safeParse(values);
            if (!success) return;
            onChange?.(data);
        }).unsubscribe;
    }, [itemValues, form.formState, onChange, form]);

    return (
        <Form {...form}>
            <form onSubmit={handleSubmit}>
                <FormField
                    control={form.control}
                    name="items"
                    render={({ field }) => (
                        <FormItem>
                            <div className="flex space-x-2">
                                <Input
                                    placeholder={placeholder}
                                    value={newItem}
                                    onChange={(e) => setNewItem(e.target.value)}
                                    className="flex-grow"
                                />
                                <Button
                                    type="button"
                                    variant="secondary"
                                    size="icon"
                                    onClick={() => {
                                        if (newItem.trim()) {
                                            field.onChange([
                                                ...(field.value || []),
                                                newItem.trim(),
                                            ]);
                                            setNewItem("");
                                        }
                                    }}
                                >
                                    <Plus className="w-4 h-4" />
                                </Button>
                            </div>

                            <div className="mt-2 w-full">
                                {field.value?.map((item, index) => (
                                    <div
                                        key={index}
                                        className="flex items-center space-x-2 justify-between mt-2"
                                    >
                                        <div className="border text-sm px-3 shadow-sm rounded-md p-2 w-full truncate">
                                            {item}
                                        </div>
                                        <Button
                                            type="button"
                                            variant="destructive"
                                            size="icon"
                                            onClick={() => {
                                                const newItems =
                                                    field.value?.filter(
                                                        (_, i) => i !== index
                                                    );
                                                field.onChange(newItems);
                                            }}
                                        >
                                            <TrashIcon className="w-4 h-4" />
                                        </Button>
                                    </div>
                                ))}
                            </div>
                            <FormMessage />
                        </FormItem>
                    )}
                />
            </form>
        </Form>
    );
}
