import { zodResolver } from "@hookform/resolvers/zod";
import { createContext, useContext, useMemo } from "react";
import { UseFormHandleSubmit, useForm, useFormContext } from "react-hook-form";

import { WebhookFormType, WebhookSchema } from "~/lib/model/webhooks";

import { Form } from "~/components/ui/form";

export type WebhookFormContextProps = {
    onSubmit: (data: WebhookFormType) => void;
    defaultValues?: Partial<WebhookFormType>;
};

type WebhookFormContextType = {
    handleSubmit: ReturnType<UseFormHandleSubmit<WebhookFormType>>;
};

const Context = createContext<WebhookFormContextType | null>(null);

export function WebhookFormContextProvider({
    children,
    onSubmit,
    defaultValues: _defaultValues,
}: WebhookFormContextProps & { children: React.ReactNode }) {
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

    const handleSubmit = form.handleSubmit(onSubmit);

    return (
        <Form {...form}>
            <Context.Provider value={{ handleSubmit }}>
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
