import { zodResolver } from "@hookform/resolvers/zod";
import { PlusIcon, TrashIcon } from "lucide-react";
import { useCallback, useEffect } from "react";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { Agent } from "~/lib/model/agents";
import { noop } from "~/lib/utils";

import { ToolCatalog } from "~/components/tools/ToolCatalog";
import { Button } from "~/components/ui/button";
import {
    Form,
    FormDescription,
    FormField,
    FormItem,
    FormLabel,
    FormMessage,
} from "~/components/ui/form";
import {
    Popover,
    PopoverContent,
    PopoverTrigger,
} from "~/components/ui/popover";

const formSchema = z.object({
    tools: z.array(z.string()),
});

export type ToolFormValues = z.infer<typeof formSchema>;

export function ToolForm({
    agent,
    onSubmit,
    onChange,
}: {
    agent: Agent;
    onSubmit?: (values: ToolFormValues) => void;
    onChange?: (values: ToolFormValues) => void;
}) {
    const form = useForm<ToolFormValues>({
        resolver: zodResolver(formSchema),
        defaultValues: { tools: agent.tools || [] },
    });

    const handleSubmit = form.handleSubmit(onSubmit || noop);

    const toolValues = form.watch("tools");

    useEffect(() => {
        return form.watch((values) => {
            const { data, success } = formSchema.safeParse(values);

            if (!success) return;

            onChange?.(data);
        }).unsubscribe;
    }, [toolValues, form.formState, onChange, form]);

    const handleToolsChange = useCallback(
        (newTools: string[]) => {
            form.setValue("tools", newTools, {
                shouldValidate: true,
                shouldDirty: true,
            });
            onChange?.({ tools: newTools });
        },
        [form, onChange]
    );

    return (
        <Form {...form}>
            <form onSubmit={handleSubmit} className="flex flex-col gap-2">
                <div className="flex justify-end w-full my-4">
                    <Popover>
                        <PopoverTrigger>
                            <Button variant="secondary">
                                <PlusIcon className="w-4 h-4 mr-2" /> Add Tool
                            </Button>
                        </PopoverTrigger>
                        <PopoverContent
                            side="left"
                            className="p-0 border-none"
                            align="end"
                            sideOffset={20}
                        >
                            <ToolCatalog
                                tools={toolValues}
                                onChangeTools={handleToolsChange}
                                invert={true}
                            />
                        </PopoverContent>
                    </Popover>
                </div>
                <FormField
                    control={form.control}
                    name="tools"
                    render={({ field }) => (
                        <FormItem>
                            <FormLabel hidden>Tools</FormLabel>
                            <FormDescription hidden>
                                The tools of the agent.
                            </FormDescription>

                            <div className="mt-2 w-full">
                                {field.value?.map((tool, index) => (
                                    <div
                                        key={index}
                                        className="flex items-center space-x-2 justify-between mt-1"
                                    >
                                        <div className="border text-sm px-3 shadow-sm rounded-md p-2 w-full">
                                            {tool}
                                        </div>
                                        <Button
                                            type="button"
                                            variant="destructive"
                                            size="icon"
                                            onClick={() => {
                                                const newTools =
                                                    field.value?.filter(
                                                        (_, i) => i !== index
                                                    );
                                                field.onChange(newTools);
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
