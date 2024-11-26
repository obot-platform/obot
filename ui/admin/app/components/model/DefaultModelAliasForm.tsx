import { useEffect, useMemo, useState } from "react";
import { useForm } from "react-hook-form";
import { toast } from "sonner";
import useSWR from "swr";

import { UpdateDefaultModelAlias } from "~/lib/model/defaultModelAliases";
import { Model, getModelUsageFromAlias } from "~/lib/model/models";
import { DefaultModelAliasApiService } from "~/lib/service/api/defaultModelAliasApiService";
import { ModelApiService } from "~/lib/service/api/modelApiService";

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
    SelectItem,
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

    const { data: models } = useSWR(
        ModelApiService.getModels.key(),
        ModelApiService.getModels
    );

    const modelUsageMap = useMemo(() => {
        return (models ?? []).reduce((acc, model) => {
            if (!acc.has(model.usage)) acc.set(model.usage, []);

            acc.get(model.usage)?.push(model);

            return acc;
        }, new Map<string, Model[]>());
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
                acc[alias.alias] = alias.model;
                return acc;
            },
            {} as Record<string, string>
        );
    }, [defaultAliases]);

    const form = useForm<Record<string, string>>({ defaultValues });

    useEffect(() => {
        return form.watch((values) => {
            const changedItems = defaultAliases?.filter(({ alias, model }) => {
                return values[alias] !== model;
            });

            if (!changedItems?.length) return;
        }).unsubscribe;
    }, [defaultAliases, form]);

    useEffect(() => {
        form.reset(defaultValues);
    }, [defaultValues, form]);

    const handleSubmit = form.handleSubmit((values) => {
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
            <form onSubmit={handleSubmit} className="space-y-6">
                {defaultAliases?.map(({ alias, model: defaultModel }) => (
                    <FormField
                        control={form.control}
                        name={alias}
                        key={alias}
                        render={({ field: { ref: _, ...field } }) => {
                            const usage = getModelUsageFromAlias(alias);
                            const modelOptions = usage
                                ? modelUsageMap.get(usage)
                                : [];

                            return (
                                <FormItem className="flex justify-between items-center space-y-0">
                                    <FormLabel>{alias}</FormLabel>

                                    <div className="flex flex-col gap-2 w-[50%]">
                                        <FormControl>
                                            <Select
                                                {...field}
                                                key={field.value}
                                                value={field.value || ""}
                                                onValueChange={field.onChange}
                                            >
                                                <SelectTrigger className="w-full">
                                                    <SelectValue
                                                        placeholder={
                                                            defaultModel
                                                        }
                                                    />
                                                </SelectTrigger>

                                                <SelectContent>
                                                    {modelOptions ? (
                                                        modelOptions.map(
                                                            (model) => (
                                                                <SelectItem
                                                                    key={
                                                                        model.id
                                                                    }
                                                                    value={
                                                                        model.id
                                                                    }
                                                                >
                                                                    {model.id}
                                                                </SelectItem>
                                                            )
                                                        )
                                                    ) : (
                                                        <SelectItem
                                                            value={defaultModel}
                                                        >
                                                            {defaultModel}
                                                        </SelectItem>
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
}

export function DefaultModelAliasFormDialog() {
    const [open, setOpen] = useState(false);

    return (
        <Dialog open={open} onOpenChange={setOpen}>
            <DialogTrigger asChild>
                <Button>Default Model Aliases</Button>
            </DialogTrigger>

            <DialogContent>
                <DialogHeader>
                    <DialogTitle>Default Model Aliases</DialogTitle>
                </DialogHeader>

                <DialogDescription>
                    Set the default model for each usage.
                </DialogDescription>

                <DefaultModelAliasForm onSuccess={() => setOpen(false)} />
            </DialogContent>
        </Dialog>
    );
}
