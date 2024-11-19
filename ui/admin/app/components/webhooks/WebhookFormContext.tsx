import { zodResolver } from "@hookform/resolvers/zod";
import { createContext, useContext, useMemo } from "react";
import { UseFormHandleSubmit, useForm, useFormContext } from "react-hook-form";
import { toast } from "sonner";
import { mutate } from "swr";

import { WebhookFormType, WebhookSchema } from "~/lib/model/webhooks";
import { WebhookApiService } from "~/lib/service/api/webhookApiService";

import { Form } from "~/components/ui/form";
import { useAsync } from "~/hooks/useAsync";

export type WebhookFormContextProps = {
    onSuccess?: () => void;
    defaultValues?: NullishPartial<WebhookFormType>;
    webhookId?: string;
};

type WebhookFormContextType = {
    handleSubmit: ReturnType<UseFormHandleSubmit<WebhookFormType>>;
    isLoading: boolean;
    isEdit: boolean;
};

const Context = createContext<WebhookFormContextType | null>(null);

export function WebhookFormContextProvider({
    children,
    defaultValues: _defaultValues,
    webhookId,
    onSuccess,
}: WebhookFormContextProps & { children: React.ReactNode }) {
    const updateWebhook = useAsync(WebhookApiService.updateWebhook, {
        onSuccess: () => {
            toast.success("Webhook updated");
        },
    });
    const createWebhook = useAsync(WebhookApiService.createWebhook, {
        onSuccess: () => {
            toast.success("Webhook created");
        },
    });

    const defaultValues = useMemo<WebhookFormType>(
        () => ({
            name: _defaultValues?.name ?? "",
            description: _defaultValues?.description ?? "",
            alias: _defaultValues?.alias ?? "",
            workflow: _defaultValues?.workflow ?? "",
            headers: _defaultValues?.headers ?? [],
            secret: _defaultValues?.secret ?? "",
            validationHeader: _defaultValues?.validationHeader ?? "",
            token: _defaultValues?.token ?? "",
        }),
        [_defaultValues]
    );

    const form = useForm<WebhookFormType>({
        resolver: zodResolver(WebhookSchema),
        defaultValues,
    });

    const handleSubmit = form.handleSubmit(async (values) => {
        if (webhookId) await updateWebhook.executeAsync(webhookId, values);
        else await createWebhook.executeAsync(values);

        mutate(WebhookApiService.getWebhooks.key());
        onSuccess?.();
    });

    return (
        <Form {...form}>
            <Context.Provider
                value={{
                    isEdit: !!webhookId,
                    handleSubmit,
                    isLoading:
                        updateWebhook.isLoading || createWebhook.isLoading,
                }}
            >
                {children}
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
