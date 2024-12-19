import { useEffect, useMemo, useState } from "react";
import { useForm } from "react-hook-form";
import { toast } from "sonner";
import useSWR from "swr";

import { UpdateDefaultModelAlias } from "~/lib/model/defaultModelAliases";
import {
    Model,
    ModelAlias,
    ModelUsage,
    filterModelsByActive,
    filterModelsByUsage,
    getModelAliasLabel,
    getModelUsageFromAlias,
    getModelUsageLabel,
} from "~/lib/model/models";
import { DefaultModelAliasApiService } from "~/lib/service/api/defaultModelAliasApiService";
import { ModelApiService } from "~/lib/service/api/modelApiService";

import { TypographyP } from "~/components/Typography";
import { SUGGESTED_MODEL_SELECTIONS } from "~/components/model/constants";
import { Button } from "~/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogHeader,
    DialogTitle,
    DialogTrigger,
} from "~/components/ui/dialog";
import {
    Form,
    FormControl,
    FormField,
    FormItem,
    FormLabel,
    FormMessage,
} from "~/components/ui/form";
import {
    Select,
    SelectContent,
    SelectGroup,
    SelectItem,
    SelectLabel,
    SelectTrigger,
    SelectValue,
} from "~/components/ui/select";
import { useAsync } from "~/hooks/useAsync";

export function DefaultModelAliasForm({
    onSuccess,
}: {
    onSuccess?: () => void;
}) {
    const { data: defaultAliases } = useSWR(
        DefaultModelAliasApiService.getAliases.key(),
        DefaultModelAliasApiService.getAliases
    );

    const sortedDefaultAliases = useMemo(() => {
        if (!defaultAliases) return null;
        return Object.values(ModelAlias)
            .map((alias) =>
                defaultAliases.find(
                    (defaultAlias) => defaultAlias.alias === alias
                )
            )
            .filter((x) => !!x);
    }, [defaultAliases]);

    const { data: models } = useSWR(
        ModelApiService.getModels.key(),
        ModelApiService.getModels
    );

    const defaultAliasMap = useMemo(() => {
        return Object.entries(SUGGESTED_MODEL_SELECTIONS).reduce(
            (acc, [alias, modelName]) => {
                acc[alias] = models?.find((model) => model.name === modelName);
                return acc;
            },
            {} as Record<string, Model | undefined>
        );
    }, [models]);

    const otherModels = useMemo(() => {
        if (!models) return [];

        return filterModelsByUsage(models, [
            ModelUsage.Unknown,
            ModelUsage.Other,
        ]);
    }, [models]);

    const update = useAsync(
        async (updates: UpdateDefaultModelAlias[]) => {
            await Promise.all(
                updates.map((update) =>
                    DefaultModelAliasApiService.updateAlias(
                        update.alias,
                        update
                    )
                )
            );
        },
        {
            onSuccess: () => {
                toast.success("Default model aliases updated");
                onSuccess?.();
            },
        }
    );

    const defaultValues = useMemo(() => {
        return defaultAliases?.reduce(
            (acc, alias) => {
                // if a default model is not set, suggest the model from the SUGGESTED_MODEL_SELECTIONS
                acc[alias.alias] =
                    alias.model || defaultAliasMap[alias.alias]?.id || "";

                return acc;
            },
            {} as Record<string, string>
        );
    }, [defaultAliases, defaultAliasMap]);

    const form = useForm<Record<string, string>>({
        defaultValues,
    });
    const { reset, watch, handleSubmit, control } = form;

    useEffect(() => {
        return watch((values) => {
            const changedItems = defaultAliases?.filter(({ alias, model }) => {
                return values[alias] !== model;
            });

            if (!changedItems?.length) return;
        }).unsubscribe;
    }, [defaultAliases, watch]);

    useEffect(() => {
        reset(defaultValues);
    }, [defaultValues, reset]);

    const handleFormSubmit = handleSubmit((values) => {
        const updates = defaultAliases
            ?.filter(({ alias, model }) => values[alias] !== model)
            .map(({ alias }) => ({
                alias,
                model: values[alias],
            }));

        update.execute(updates ?? []);
    });

    return (
        <Form {...form}>
            <form onSubmit={handleFormSubmit} className="space-y-6">
                {sortedDefaultAliases?.map(({ alias, model: defaultModel }) => (
                    <FormField
                        control={control}
                        name={alias}
                        key={alias}
                        render={({ field: { ref: _, ...field } }) => {
                            const usage =
                                getModelUsageFromAlias(alias) ??
                                ModelUsage.Unknown;

                            const activeModelOptions = filterModelsByActive(
                                filterModelsByUsage(models ?? [], usage)
                            );

                            return (
                                <FormItem className="flex justify-between items-center space-y-0">
                                    <FormLabel>
                                        {getModelAliasLabel(alias)}
                                    </FormLabel>

                                    <div className="flex flex-col gap-2 w-[50%]">
                                        <FormControl>
                                            <Select
                                                {...field}
                                                key={field.value}
                                                value={field.value || ""}
                                                onValueChange={field.onChange}
                                            >
                                                <SelectTrigger>
                                                    <SelectValue
                                                        placeholder={
                                                            defaultModel
                                                        }
                                                    />
                                                </SelectTrigger>

                                                <SelectContent>
                                                    {renderSelectContent(
                                                        activeModelOptions,
                                                        defaultModel,
                                                        usage,
                                                        alias
                                                    )}
                                                </SelectContent>
                                            </Select>
                                        </FormControl>

                                        <FormMessage />
                                    </div>
                                </FormItem>
                            );
                        }}
                    />
                ))}

                <Button
                    type="submit"
                    className="w-full"
                    disabled={update.isLoading}
                    loading={update.isLoading}
                >
                    Save Changes
                </Button>
            </form>
        </Form>
    );

    function renderSelectContent(
        modelOptions: Model[] | undefined,
        defaultModel: string,
        usage: ModelUsage,
        aliasFor: ModelAlias
    ) {
        if (!modelOptions) {
            if (!defaultModel)
                return (
                    <TypographyP className="p-2 text-muted-foreground">
                        No Models Available.
                    </TypographyP>
                );
            return <SelectItem value={defaultModel}>{defaultModel}</SelectItem>;
        }

        return (
            <>
                <SelectGroup className="relative">
                    <SelectLabel>{getModelUsageLabel(usage)}</SelectLabel>

                    {modelOptions.map((model) => (
                        <SelectItem key={model.id} value={model.id}>
                            {getModelOptionLabel(model, aliasFor)}
                        </SelectItem>
                    ))}
                </SelectGroup>

                {otherModels.length > 0 && (
                    <SelectGroup>
                        <SelectLabel>Other</SelectLabel>

                        {otherModels.map((model) => (
                            <SelectItem key={model.id} value={model.id}>
                                {model.name || model.id}
                                {" - "}
                                <span className="text-muted-foreground">
                                    {model.modelProvider}
                                </span>
                            </SelectItem>
                        ))}
                    </SelectGroup>
                )}
            </>
        );
    }
}

function getModelOptionLabel(model: Model, aliasFor: ModelAlias) {
    // if the model name is the same as the suggested model name, show that it's suggested
    const suggestionName = SUGGESTED_MODEL_SELECTIONS[aliasFor];
    return (
        <>
            {model.name || model.id}{" "}
            {suggestionName === model.name && (
                <span className="text-muted-foreground">(Suggested)</span>
            )}
            {" - "}
            <span className="text-muted-foreground">{model.modelProvider}</span>
        </>
    );
}

export function DefaultModelAliasFormDialog({
    disabled,
}: {
    disabled?: boolean;
}) {
    const [open, setOpen] = useState(false);

    return (
        <Dialog open={open} onOpenChange={setOpen}>
            <DialogTrigger asChild>
                <Button disabled={disabled}>Set Default Models</Button>
            </DialogTrigger>

            <DialogContent className="max-w-2xl">
                <DialogHeader>
                    <DialogTitle>Default Model Aliases</DialogTitle>
                </DialogHeader>

                <DialogDescription>
                    When no model is specified, a default model is used for
                    creating a new agent, workflow, or working with some tools,
                    etc. Select your default models for the usage types below.
                </DialogDescription>

                <DefaultModelAliasForm onSuccess={() => setOpen(false)} />
            </DialogContent>
        </Dialog>
    );
}
