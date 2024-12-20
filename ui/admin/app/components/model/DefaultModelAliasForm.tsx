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

import { ComboBox } from "~/components/composed/ComboBox";
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
                                            <ComboBox
                                                emptyLabel="No Models Available."
                                                placeholder=""
                                                onChange={(value) =>
                                                    field.onChange(
                                                        value?.id ?? ""
                                                    )
                                                }
                                                options={getOptionsByUsageAndProvider(
                                                    activeModelOptions,
                                                    usage
                                                )}
                                                suggested={getSuggested(alias)}
                                                value={
                                                    field.value
                                                        ? models?.find(
                                                              (m) =>
                                                                  m.id ===
                                                                  field.value
                                                          )
                                                        : models?.find(
                                                              (m) =>
                                                                  m.name ===
                                                                  defaultModel
                                                          )
                                                }
                                            />
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

    function getSuggested(alias: ModelAlias) {
        return alias && SUGGESTED_MODEL_SELECTIONS[alias]
            ? [SUGGESTED_MODEL_SELECTIONS[alias]]
            : [];
    }

    function getOptionsByUsageAndProvider(
        modelOptions: Model[] | undefined,
        usage: ModelUsage
    ) {
        if (!modelOptions) return [];

        const usageGroupName = getModelUsageLabel(usage);
        const usageModelProviderGroups =
            getModelOptionsByModelProvider(modelOptions);
        const otherModelProviderGroups =
            getModelOptionsByModelProvider(otherModels);
        const usageGroup = {
            heading: usageGroupName,
            value: usageModelProviderGroups,
        };

        if (
            usageModelProviderGroups.length === 0 &&
            otherModelProviderGroups.length === 0
        ) {
            return [];
        }

        return otherModelProviderGroups.length > 0
            ? [
                  usageGroup,
                  {
                      heading: "Other",
                      value: otherModelProviderGroups,
                  },
              ]
            : [usageGroup];
    }
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

export function getModelOptionsByModelProvider(models: Model[]) {
    const byModelProviderGroups = filterModelsByActive(models).reduce(
        (acc, model) => {
            acc[model.modelProvider] = acc[model.modelProvider] || [];
            acc[model.modelProvider].push(model);
            return acc;
        },
        {} as Record<string, Model[]>
    );

    return Object.entries(byModelProviderGroups).map(
        ([modelProvider, models]) => {
            const sorted = models.sort((a, b) =>
                (a.name ?? "").localeCompare(b.name ?? "")
            );
            return {
                heading: modelProvider,
                value: sorted,
            };
        }
    );
}
