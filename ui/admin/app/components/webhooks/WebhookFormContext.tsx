import { zodResolver } from "@hookform/resolvers/zod";
import { createContext, useContext, useEffect, useMemo, useState } from "react";
import { UseFormHandleSubmit, useForm, useFormContext } from "react-hook-form";
import { toast } from "sonner";
import { mutate } from "swr";
import { z } from "zod";

import { Webhook, WebhookFormType, WebhookSchema } from "~/lib/model/webhooks";
import { WebhookApiService } from "~/lib/service/api/webhookApiService";

import { Form } from "~/components/ui/form";
import {
    WebhookConfirmation,
    WebhookConfirmationProps,
} from "~/components/webhooks/WebhookConfirmation";
import { useAsync } from "~/hooks/useAsync";

export type WebhookFormContextProps = {
    webhook?: Webhook;
};

type WebhookFormContextType = {
    handleSubmit: ReturnType<UseFormHandleSubmit<WebhookFormType>>;
    isLoading: boolean;
    error?: unknown;
    isEdit: boolean;
    hasToken: boolean;
    hasSecret: boolean;
};

const Context = createContext<WebhookFormContextType | null>(null);

const CreateSchema = WebhookSchema;
const EditSchema = WebhookSchema.extend({
    secret: z.string(),
});

export function WebhookFormContextProvider({
    children,
    webhook,
}: WebhookFormContextProps & { children: React.ReactNode }) {
    const webhookId = webhook?.id;

    const [webhookConfirmation, showWebhookConfirmation] =
        useState<WebhookConfirmationProps | null>(null);

    const action = useAsync(handler);

    const defaultValues = useMemo<WebhookFormType>(
        () => ({
            name: webhook?.name ?? "",
            description: webhook?.description ?? "",
            alias: webhook?.alias ?? "",
            workflow: webhook?.workflow ?? "",
            headers: webhook?.headers ?? ["User-Agent", "X-GitHub-Event"],
            validationHeader: webhook?.validationHeader ?? "",
            secret: "",
            token: "",
            removeToken: false,
            removeSecret: false,
        }),
        [webhook]
    );

    const form = useForm<WebhookFormType>({
        resolver: zodResolver(webhookId ? EditSchema : CreateSchema),
        defaultValues,
    });

    useEffect(() => {
        form.reset(defaultValues);
    }, [defaultValues, form]);

    const handleSubmit = form.handleSubmit(async (values) => {
        const { data, error } = await action.executeAsync(webhookId, values);

        if (error) {
            if (error instanceof Error) toast.error(error.message);
            else toast.error("Failed to save webhook");

            return;
        }

        mutate(WebhookApiService.getWebhooks.key());
        showWebhookConfirmation({
            webhook: data,
            secret: values.secret,
            token: values.token,
            original: webhook,
            tokenRemoved: values.removeToken,
            secretRemoved: !values.secret && !values.validationHeader,
        });
    });

    return (
        <Form {...form}>
            <Context.Provider
                value={{
                    error: action.error,
                    isEdit: !!webhookId,
                    hasSecret: !!webhook?.secret,
                    hasToken: !!webhook?.hasToken,
                    handleSubmit,
                    isLoading: action.isLoading,
                }}
            >
                {children}

                {webhookConfirmation && (
                    <WebhookConfirmation {...webhookConfirmation} />
                )}
            </Context.Provider>
        </Form>
    );
}

export function useWebhookFormContext() {
    const form = useFormContext<WebhookFormType>();

    const helpers = useContext(Context);

    if (!helpers) {
        throw new Error(
            "useWebhookFormContext must be used within a WebhookFormContextProvider"
        );
    }

    if (!form) {
        throw new Error(
            "useWebhookFormContext must be used within a WebhookFormContextProvider"
        );
    }

    return { form, ...helpers };
}

async function handleRemoveToken(threadId: string) {
    const res = await WebhookApiService.removeWebhookToken(threadId);
    toast.success("Token removed");
    return res;
}

async function handler(
    threadId: string | undefined,
    { removeToken, removeSecret, ...values }: WebhookFormType
) {
    if (threadId) {
        const res = await WebhookApiService.updateWebhook(threadId, {
            ...values,
            ...(removeSecret ? { secret: "", validationHeader: "" } : {}),
        });
        toast.success("Webhook updated");

        if (removeToken) return await handleRemoveToken(threadId);

        return res;
    }

    const res = await WebhookApiService.createWebhook(values);

    toast.success("Webhook created");

    return res;
}
