import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect, useMemo } from "react";
import { useFieldArray, useForm } from "react-hook-form";
import { z } from "zod";

import { ToolEntry } from "~/components/agent/ToolEntry";
import { ToolCatalogDialog } from "~/components/tools/ToolCatalog";
import { Form } from "~/components/ui/form";

const formSchema = z.object({
    tools: z.array(z.object({ value: z.string() })),
});

type BasicToolFormValues = z.infer<typeof formSchema>;

type Tools = { tools: string[] };

export function BasicToolForm({
    defaultValues: _defaultValues,
    onChange,
}: {
    defaultValues?: Partial<Tools>;
    onSubmit?: (values: Tools) => void;
    onChange?: (values: Tools) => void;
}) {
    const defaultValues = useMemo(() => {
        return {
            tools:
                _defaultValues?.tools?.map((tool) => ({ value: tool })) || [],
        };
    }, [_defaultValues]);

    const form = useForm<BasicToolFormValues>({
        resolver: zodResolver(formSchema),
        defaultValues: { tools: defaultValues?.tools || [] },
    });
    const { getValues, reset } = form;

    useEffect(() => {
        const unchanged = compareArrays(
            defaultValues?.tools.map(({ value }) => value) || [],
            getValues().tools.map(({ value }) => value)
        );

        if (unchanged) return;

        reset({ tools: defaultValues?.tools || [] });
    }, [defaultValues, getValues, reset]);

    const toolArr = useFieldArray({ control: form.control, name: "tools" });

    useEffect(() => {
        return form.watch((values) => {
            const { data, success } = formSchema.safeParse(values);

            if (!success) return;

            onChange?.({ tools: data.tools.map((t) => t.value) });
        }).unsubscribe;
    }, [form, onChange]);

    const removeTools = (toolsToRemove: string[]) => {
        const indexes = toolsToRemove
            .map((tool) => toolArr.fields.findIndex((t) => t.value === tool))
            .filter((index) => index !== -1);

        toolArr.remove(indexes);
    };

    const addTool = (tool: string) => {
        toolArr.append({ value: tool });
    };

    return (
        <Form {...form}>
            <div className="flex flex-col gap-2">
                <div className="flex flex-col gap-1 w-full overflow-y-auto">
                    {toolArr.fields.map((field) => (
                        <ToolEntry
                            key={field.id}
                            tool={field.value}
                            onDelete={() => removeTools([field.value])}
                        />
                    ))}
                </div>

                <div className="flex justify-end">
                    <ToolCatalogDialog
                        tools={toolArr.fields.map((field) => field.value)}
                        onAddTool={addTool}
                        onRemoveTools={removeTools}
                    />
                </div>
            </div>
        </Form>
    );
}

function compareArrays(a: string[], b: string[]) {
    const aSet = new Set(a);

    if (aSet.size !== b.length) return false;

    return b.every((tool) => aSet.has(tool));
}
