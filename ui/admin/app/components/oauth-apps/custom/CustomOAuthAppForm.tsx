import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect, useMemo, useState } from "react";
import { useForm } from "react-hook-form";
import { mutate } from "swr";
import { z } from "zod";

import { OAuthApp } from "~/lib/model/oauthApps";
import { OAuthProvider } from "~/lib/model/oauthApps/oauth-helpers";
import { ConflictError } from "~/lib/service/api/apiErrors";
import { OauthAppService } from "~/lib/service/api/oauthAppService";
import { ErrorService } from "~/lib/service/errorService";

import { TypographySmall } from "~/components/Typography";
import { CopyText } from "~/components/composed/CopyText";
import { ControlledInput } from "~/components/form/controlledInputs";
import { Button } from "~/components/ui/button";
import { Form } from "~/components/ui/form";
import { useAsync } from "~/hooks/useAsync";

const Step = {
    NAME: 1,
    INFO: 2,
} as const;
type Step = (typeof Step)[keyof typeof Step];

const nameSchema = z.object({
    name: z.string().min(1, "Required"),
    integration: z
        .string()
        .min(1, "Required")
        .regex(
            /^[a-z0-9-]+$/,
            "Must contain only lowercase letters, numbers, and dashes (-)"
        ),
    authURL: z.string().url("Invalid URL").min(1, "Required"),
    tokenURL: z.string().url("Invalid URL").min(1, "Required"),
});

const finalSchema = nameSchema.extend({
    clientID: z.string().min(1, "Required"),
    clientSecret: z.string().min(1, "Required"),
});

const SchemaMap = {
    [Step.NAME]: nameSchema,
    [Step.INFO]: finalSchema,
} as const;

type FormData = z.infer<typeof finalSchema>;

type CustomOAuthAppFormProps = {
    defaultData?: OAuthApp;
    onComplete: () => void;
    onCancel?: () => void;
    defaultStep?: Step;
};

export function CustomOAuthAppForm({
    defaultData,
    onComplete,
    onCancel,
    defaultStep = Step.NAME,
}: CustomOAuthAppFormProps) {
    const createApp = useAsync(OauthAppService.createOauthApp, {
        onSuccess: () => mutate(OauthAppService.getOauthApps.key()),
    });

    const updateApp = useAsync(OauthAppService.updateOauthApp, {
        onSuccess: () => {
            mutate(OauthAppService.getOauthApps.key());
            onComplete();
        },
        onError: ErrorService.toastError,
    });

    const initialIsEdit = !!defaultData;

    const app = defaultData || createApp.data;

    const isEdit = !!app;

    const [step, setStep] = useState<Step>(defaultStep);

    const defaultValues = useMemo(() => {
        if (defaultData) return { ...defaultData, clientSecret: "" };

        return Object.keys(finalSchema.shape).reduce((acc, _key) => {
            const key = _key as keyof FormData;
            acc[key] = "";

            return acc;
        }, {} as FormData);
    }, [defaultData]);

    const getStepSchema = (step: Step) => {
        if (step === Step.INFO && initialIsEdit)
            // clientSecret is not required for editing
            // leaving secret empty indicates that it's unchanged
            return finalSchema.extend({
                clientSecret: z.string(),
            });

        return SchemaMap[step];
    };

    const form = useForm<FormData>({
        resolver: zodResolver(getStepSchema(step)),
        defaultValues,
    });

    const {
        isFinalStep,
        nextLabel,
        prevLabel,
        isLoading,
        onBack,
        onNext,
        disableSubmit,
    } = getStepInfo(step);

    useEffect(() => {
        form.reset(defaultValues);
    }, [defaultValues, form]);

    const handleSubmit = form.handleSubmit(async (data) => {
        if (step === Step.NAME) {
            // try creating the app if there is no existing app
            if (!isEdit) {
                const { error } = await createApp.executeAsync({
                    type: OAuthProvider.Custom,
                    global: true,
                    ...data,
                });

                if (error instanceof ConflictError)
                    form.setError("integration", { message: error.message });

                // do not proceed to the next step if there's an error
                if (error) return;
            }
        }

        if (!isFinalStep) {
            onNext();
            return;
        }

        if (!app) {
            // should never happen
            // indicates that step 1 was not completed yet somehow we're on step 2
            throw new Error("App is required");
        }

        updateApp.execute(app.id, { ...data });
    });

    // once a user touches the integration field, we don't auto-derive it from the name
    const deriveIntegrationFromName =
        !initialIsEdit && !form.formState.touchedFields.integration;

    return (
        <Form {...form}>
            <form
                onSubmit={handleSubmit}
                className="space-y-4 overflow-x-hidden p-1"
            >
                {step === Step.NAME && (
                    <>
                        <ControlledInput
                            control={form.control}
                            onChange={(e) => {
                                if (deriveIntegrationFromName) {
                                    form.setValue(
                                        "integration",
                                        convertToIntegration(e.target.value)
                                    );
                                }
                            }}
                            name="name"
                            label="Name"
                        />

                        <ControlledInput
                            control={form.control}
                            description="This value will be used to link tools to your OAuth app"
                            name="integration"
                            label="Integration"
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
                    </>
                )}

                {step === Step.INFO && app && (
                    <>
                        <div className="flex flex-col gap-2">
                            <TypographySmall>Redirect URL</TypographySmall>

                            <CopyText
                                text={app.links.redirectURL}
                                className="w-full justify-between"
                            />
                        </div>

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
                            placeholder={
                                initialIsEdit ? "(Unchanged)" : undefined
                            }
                        />
                    </>
                )}

                <div className="flex gap-2">
                    <Button
                        className="flex-1 w-full"
                        type="button"
                        variant="secondary"
                        onClick={onBack}
                    >
                        {prevLabel}
                    </Button>

                    <Button
                        className="flex-1 w-full"
                        type="submit"
                        loading={isLoading}
                        disabled={disableSubmit}
                    >
                        {nextLabel}
                    </Button>
                </div>
            </form>
        </Form>
    );

    function getStepInfo(step: Step) {
        if (step === Step.INFO) {
            return {
                isFinalStep: true,
                nextLabel: "Submit",
                prevLabel: "Back",
                onBack: () => setStep((prev) => (prev - 1) as Step),
                isLoading: updateApp.isLoading,
                disableSubmit:
                    !form.formState.isValid || !form.formState.isDirty,
            } as const;
        }

        return {
            nextLabel: "Next",
            prevLabel: "Cancel",
            onBack: onCancel,
            onNext: () => setStep((prev) => (prev + 1) as Step),
            isLoading: createApp.isLoading,
        } as const;
    }
}

function convertToIntegration(name: string) {
    return name.toLowerCase().replace(/[\s\W]+/g, "-");
}
