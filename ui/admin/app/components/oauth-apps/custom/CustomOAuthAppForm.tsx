import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect, useMemo } from "react";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { OAuthApp } from "~/lib/model/oauthApps";

import { ControlledInput } from "~/components/form/controlledInputs";
import { Button } from "~/components/ui/button";
import { Form } from "~/components/ui/form";

const schema = z.object({
    name: z.string().min(1),
    integration: z.string().min(1),
    clientID: z.string().min(1),
    clientSecret: z.string().min(1).optional(),
    authURL: z.string().min(1),
    tokenURL: z.string().min(1),
});

type FormData = z.infer<typeof schema>;

type CustomOAuthAppFormProps = {
    app?: OAuthApp;
    onSubmit: (app: FormData) => void;
};

export function CustomOAuthAppForm({ app, onSubmit }: CustomOAuthAppFormProps) {
    const isEdit = !!app;

    const defaultValues = useMemo(() => {
        if (app)
            return {
                ...app,
                clientSecret: SECRET_PLACEHOLDER,
            };

        return Object.keys(schema.shape).reduce((acc, _key) => {
            const key = _key as keyof FormData;
            acc[key] = "";

            return acc;
        }, {} as FormData);
    }, [app]);

    const form = useForm<FormData>({
        resolver: zodResolver(schema),
        defaultValues,
    });

    useEffect(() => {
        form.reset(defaultValues);
    }, [defaultValues, form]);

    const handleSubmit = form.handleSubmit((data) => {
        if (isEdit && data.clientSecret === SECRET_PLACEHOLDER) {
            delete data.clientSecret;
        }

        onSubmit(data);
    });

    const onFocusSecret = () => {
        if (!isEdit) return;

        if (form.getValues("clientSecret") === SECRET_PLACEHOLDER) {
            form.setValue("clientSecret", "");
        }
    };

    const onBlurSecret = () => {
        if (!isEdit) return;

        if (form.getValues("clientSecret") === "") {
            form.setValue("clientSecret", SECRET_PLACEHOLDER);
        }
    };

    return (
        <Form {...form}>
            <form onSubmit={handleSubmit} className="space-y-4">
                <ControlledInput
                    control={form.control}
                    name="name"
                    label="Name"
                />

                <ControlledInput
                    control={form.control}
                    name="integration"
                    label="Integration"
                />

                <ControlledInput
                    control={form.control}
                    name="clientID"
                    label="Client ID"
                />

                <ControlledInput
                    control={form.control}
                    name="clientSecret"
                    label="Client Secret"
                    data-1p-ignore
                    type="password"
                    onFocusCapture={onFocusSecret}
                    onBlurCapture={onBlurSecret}
                />

                <ControlledInput
                    control={form.control}
                    name="authURL"
                    label="Authorization URL"
                />

                <ControlledInput
                    control={form.control}
                    name="tokenURL"
                    label="Token URL"
                />

                <Button className="w-full" type="submit">
                    Submit
                </Button>
            </form>
        </Form>
    );
}

const SECRET_PLACEHOLDER = crypto.randomUUID();
