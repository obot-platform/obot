import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect, useMemo } from "react";
import { useForm } from "react-hook-form";
import Markdown from "react-markdown";
import rehypeExternalLinks from "rehype-external-links";

import { OAuthAppParams } from "~/lib/model/oauthApps";
import {
    OAuthFormStep,
    OAuthProvider,
} from "~/lib/model/oauthApps/oauth-helpers";
import { OauthAppService } from "~/lib/service/api/oauthAppService";
import { cn } from "~/lib/utils";

import { ControlledInput } from "~/components/form/controlledInputs";
import { Button } from "~/components/ui/button";
import { Form } from "~/components/ui/form";
import { useOAuthAppInfo } from "~/hooks/oauthApps";

import { CopyText } from "../composed/CopyText";
import { CustomMarkdownComponents } from "../react-markdown";

type OAuthAppFormProps = {
    type: OAuthProvider;
    onSubmit: (data: OAuthAppParams) => void;
};

export function OAuthAppForm({ type, onSubmit }: OAuthAppFormProps) {
    const spec = useOAuthAppInfo(type);
    useEffect(() => {
        OauthAppService.getSupportedOauthAppTypes();
    }, []);

    const isEdit = !!spec.customApp;

    const fields = useMemo(() => {
        return Object.entries(spec.schema.shape).map(([key]) => ({
            key: key as keyof OAuthAppParams,
            label: spec.labels[key],
        }));
    }, [spec.schema, spec.labels]);

    const defaultValues = useMemo(() => {
        const app = spec.customApp;

        return fields.reduce((acc, { key }) => {
            acc[key] = app?.[key] ?? "";

            // if editing, use placeholder to show secret value exists
            // use a uuid to ensure it never collides with a real secret
            if (key === "clientSecret" && isEdit) {
                acc.clientSecret = SECRET_PLACEHOLDER;
            }

            return acc;
        }, {} as OAuthAppParams);
    }, [fields, spec.customApp, isEdit]);

    const form = useForm({
        defaultValues,
        resolver: zodResolver(spec.schema),
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
                {spec.steps.map(renderStep)}

                <Button type="submit">Submit</Button>
            </form>
        </Form>
    );

    function renderStep(step: OAuthFormStep) {
        switch (step.type) {
            case "instruction":
                return (
                    <Markdown
                        className={cn(
                            "flex-auto max-w-full prose overflow-x-auto dark:prose-invert prose-pre:whitespace-pre-wrap prose-pre:break-words prose-thead:text-left prose-img:rounded-xl prose-img:shadow-lg break-words"
                        )}
                        components={CustomMarkdownComponents}
                        rehypePlugins={[
                            [rehypeExternalLinks, { target: "_blank" }],
                        ]}
                    >
                        {step.text}
                    </Markdown>
                );
            case "input": {
                const isRequired = !spec.schema.shape[step.input].isOptional();

                const label = isRequired
                    ? `${spec.labels[step.input]} *`
                    : spec.labels[step.input];

                return (
                    <ControlledInput
                        key={step.input}
                        name={step.input as keyof OAuthAppParams}
                        label={label}
                        control={form.control}
                        {...(step.input === "clientSecret" && {
                            onBlur: onBlurClientSecret,
                            onFocus: onFocusClientSecret,
                            type: "password",
                        })}
                    />
                );
            }
            case "copy": {
                return <CopyText text={step.text} className="justify-center" />;
            }
        }
    }

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
