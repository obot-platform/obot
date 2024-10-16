import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect, useMemo } from "react";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { OAuthApp, OAuthAppInfo, OAuthAppParams } from "~/lib/model/oauthApps";

import { ControlledInput } from "~/components/form/controlledInputs";
import { Button } from "~/components/ui/button";
import { Form } from "~/components/ui/form";

type OAuthAppFormProps = {
    appSpec: OAuthAppInfo;
    onSubmit: (data: OAuthAppParams) => void;
    oauthApp?: OAuthApp;
};

export function OAuthAppForm({
    appSpec,
    onSubmit,
    oauthApp,
}: OAuthAppFormProps) {
    const isEdit = !!oauthApp;

    const fields = useMemo(() => {
        return Object.entries(appSpec.parameters).map(([key, label]) => ({
            key: key as keyof OAuthAppParams,
            label,
        }));
    }, [appSpec.parameters]);

    const schema = useMemo(() => {
        return z.object(
            fields.reduce(
                (acc, { key }) => {
                    acc[key] = z.string();
                    return acc;
                },
                {} as Record<keyof OAuthAppParams, z.ZodString>
            )
        );
    }, [fields]);

    const defaultValues = useMemo(() => {
        return fields.reduce((acc, { key }) => {
            acc[key] = oauthApp?.[key] ?? "";

            // if editing, use placeholder to show secret value exists
            // use a uuid to ensure it never collides with a real secret
            if (key === "clientSecret" && isEdit) {
                acc.clientSecret = SECRET_PLACEHOLDER;
            }

            return acc;
        }, {} as OAuthAppParams);
    }, [fields, oauthApp, isEdit]);

    const form = useForm({
        defaultValues,
        resolver: zodResolver(schema),
    });

    useEffect(() => {
        form.reset(defaultValues);
    }, [defaultValues, form]);

    const handleSubmit = form.handleSubmit((data) => {
        const { clientSecret, ...rest } = data;

        // if the user skips editing the client secret, we don't want to submit an empty string
        // because that will clear it out on the server
        if (isEdit && clientSecret === SECRET_PLACEHOLDER) {
            onSubmit(rest);
        } else {
            onSubmit(data);
        }
    });
    return (
        <Form {...form}>
            <form onSubmit={handleSubmit} className="flex flex-col gap-4">
                {fields.map(({ key, label }) => (
                    <ControlledInput
                        key={key}
                        name={key}
                        label={label}
                        control={form.control}
                        {...(key === "clientSecret"
                            ? {
                                  onBlur: onBlurClientSecret,
                                  onFocus: onFocusClientSecret,
                                  type: "password",
                              }
                            : {})}
                    />
                ))}

                <Button type="submit">Submit</Button>
            </form>
        </Form>
    );

    function onBlurClientSecret() {
        if (!isEdit) return;

        const { clientSecret } = form.getValues();

        if (!clientSecret) {
            form.setValue("clientSecret", SECRET_PLACEHOLDER);
        }
    }

    function onFocusClientSecret() {
        if (!isEdit) return;

        const { clientSecret } = form.getValues();

        if (clientSecret === SECRET_PLACEHOLDER) {
            form.setValue("clientSecret", "");
        }
    }
}

const SECRET_PLACEHOLDER = crypto.randomUUID();
