import { zodResolver } from "@hookform/resolvers/zod";
import { ReactNode, useEffect, useMemo } from "react";
import { useFieldArray, useForm } from "react-hook-form";
import { z } from "zod";

import { Agent } from "~/lib/model/agents";
import { noop } from "~/lib/utils";

import { TypographyP, TypographySmall } from "~/components/Typography";
import { ToolEntry } from "~/components/agent/ToolEntry";
import { ToolCatalogDialog } from "~/components/tools/ToolCatalog";
import { Form } from "~/components/ui/form";
import { Switch } from "~/components/ui/switch";
import {
    Tooltip,
    TooltipContent,
    TooltipTrigger,
} from "~/components/ui/tooltip";

const ToolVariant = {
    FIXED: "fixed",
    DEFAULT: "default",
    AVAILABLE: "available",
} as const;
type ToolVariant = (typeof ToolVariant)[keyof typeof ToolVariant];
const formSchema = z.object({
    tools: z.array(
        z.object({
            tool: z.string(),
            variant: z.enum([
                ToolVariant.FIXED,
                ToolVariant.DEFAULT,
                ToolVariant.AVAILABLE,
            ] as const),
        })
    ),
});

export type ToolFormValues = z.infer<typeof formSchema>;

export function ToolForm({
    agent,
    onSubmit,
    onChange,
    renderActions,
}: {
    agent: Agent;
    onSubmit?: (values: ToolFormValues) => void;
    onChange?: (values: ToolFormValues) => void;
    renderActions?: (tool: string) => ReactNode;
}) {
    const defaultValues = useMemo(() => {
        return {
            tools: [
                ...(agent.tools ?? []).map((tool) => ({
                    tool,
                    variant: ToolVariant.FIXED,
                })),
                ...(agent.defaultThreadTools ?? []).map((tool) => ({
                    tool,
                    variant: ToolVariant.DEFAULT,
                })),
                ...(agent.availableThreadTools ?? []).map((tool) => ({
                    tool,
                    variant: ToolVariant.AVAILABLE,
                })),
            ],
        };
    }, [agent]);

    const form = useForm<ToolFormValues>({
        resolver: zodResolver(formSchema),
        defaultValues,
    });
    const { control, handleSubmit, getValues, reset, watch } = form;

    useEffect(() => {
        const unchanged = compareArrays(
            defaultValues.tools.map((x) => x.tool),
            getValues("tools").map((x) => x.tool)
        );

        if (unchanged) return;

        reset(defaultValues);
    }, [defaultValues, reset, getValues]);

    const toolFields = useFieldArray({
        control,
        name: "tools",
    });

    useEffect(() => {
        return watch((values) => {
            const { data, success } = formSchema.safeParse(values);

            if (!success) return;

            onChange?.(data);
        }).unsubscribe;
    }, [watch, onChange]);

    const [allTools, fixedFields, userFields] = useMemo(() => {
        return [
            toolFields.fields.map(({ tool }) => tool),
            toolFields.fields?.filter(
                (field) => field.variant === ToolVariant.FIXED
            ),
            toolFields.fields?.filter(
                (field) => field.variant !== ToolVariant.FIXED
            ),
        ];
    }, [toolFields]);

    const removeTools = (tools: string[]) => {
        const indexes = tools
            .map((tool) => toolFields.fields.findIndex((t) => t.tool === tool))
            .filter((index) => index !== -1);

        toolFields.remove(indexes);
    };

    const updateVariant = (tool: string, variant: ToolVariant) =>
        toolFields.update(
            toolFields.fields.findIndex((t) => t.tool === tool),
            { tool, variant }
        );

    const updateTools = (tools: string[], variant: ToolVariant) => {
        const removedToolIndexes = toolFields.fields
            .filter((field) => !tools.includes(field.tool))
            .map((item) => toolFields.fields.indexOf(item));

        const addedTools = tools.filter(
            (tool) => !toolFields.fields.some((field) => field.tool === tool)
        );

        toolFields.remove(removedToolIndexes);

        for (const tool of addedTools) {
            toolFields.append({ tool, variant });
        }
    };

    return (
        <Form {...form}>
            <form
                onSubmit={handleSubmit(onSubmit || noop)}
                className="flex flex-col gap-2"
            >
                <TypographyP className="flex justify-between items-end font-normal">
                    Agent Tools
                </TypographyP>

                <TypographySmall className="text-muted-foreground">
                    These tools are essential for the agent&apos;s core
                    functionality and are always enabled.
                </TypographySmall>

                <div className="mt-2 w-full overflow-y-auto">
                    {fixedFields.map((field) => (
                        <ToolEntry
                            key={field.id}
                            tool={field.tool}
                            onDelete={() => removeTools([field.tool])}
                            actions={renderActions?.(field.tool)}
                        />
                    ))}
                </div>

                <div className="flex justify-end">
                    <ToolCatalogDialog
                        tools={allTools}
                        onUpdateTools={(tools) =>
                            updateTools(tools, ToolVariant.FIXED)
                        }
                    />
                </div>

                <TypographyP className="flex justify-between items-end font-normal mt-4">
                    User Tools
                </TypographyP>

                <TypographySmall className="text-muted-foreground">
                    Optional tools users can turn on or off. Use the toggle to
                    set whether they&apos;re active by default for the agent.
                </TypographySmall>

                <div className="mt-2 w-full overflow-y-auto">
                    {userFields.map((field) => (
                        <ToolEntry
                            key={field.id}
                            tool={field.tool}
                            onDelete={() => removeTools([field.tool])}
                            actions={
                                <>
                                    {renderActions?.(field.tool)}
                                    <Tooltip>
                                        <TooltipTrigger asChild>
                                            <div>
                                                <Switch
                                                    checked={
                                                        field.variant ===
                                                        ToolVariant.DEFAULT
                                                    }
                                                    onCheckedChange={(
                                                        checked
                                                    ) =>
                                                        updateVariant(
                                                            field.tool,
                                                            checked
                                                                ? ToolVariant.DEFAULT
                                                                : ToolVariant.AVAILABLE
                                                        )
                                                    }
                                                />
                                            </div>
                                        </TooltipTrigger>

                                        <TooltipContent>
                                            {field.variant ===
                                            ToolVariant.DEFAULT
                                                ? "Active by Default"
                                                : "Inactive by Default"}
                                        </TooltipContent>
                                    </Tooltip>
                                </>
                            }
                        />
                    ))}
                </div>

                <div className="flex justify-end">
                    <ToolCatalogDialog
                        tools={allTools}
                        onUpdateTools={(tools) =>
                            updateTools(tools, ToolVariant.DEFAULT)
                        }
                        className="w-auto"
                    />
                </div>
            </form>
        </Form>
    );
}

function compareArrays(a: string[], b: string[]) {
    const aSet = new Set(a);

    if (aSet.size !== b.length) return false;

    return b.every((tool) => aSet.has(tool));
}
