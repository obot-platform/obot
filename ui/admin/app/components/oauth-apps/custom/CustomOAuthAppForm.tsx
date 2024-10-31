import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect, useMemo, useState } from "react";
import { useForm } from "react-hook-form";
import { toast } from "sonner";
import { z } from "zod";

import { OAuthApp } from "~/lib/model/oauthApps";
import { OAuthProvider } from "~/lib/model/oauthApps/oauth-helpers";
import { OauthAppService } from "~/lib/service/api/oauthAppService";
import { ErrorService } from "~/lib/service/errorService";

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
});

const finalSchema = nameSchema.extend({
    clientID: z.string().min(1, "Required"),
    clientSecret: z.string().min(1, "Required"),
    authURL: z.string().min(1, "Required"),
    tokenURL: z.string().min(1, "Required"),
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
    const createApp = useAsync(OauthAppService.createOauthApp);

    const updateApp = useAsync(OauthAppService.updateOauthApp, {
        onSuccess: onComplete,
        onError: ErrorService.toastError,
    });

    const initialIsEdit = !!defaultData;

    const app = defaultData || createApp.data;

    const isEdit = !!app;

    const [step, setStep] = useState<Step>(defaultStep);
    const { isFinal, nextLabel, prevLabel, onBack, onNext } = getStepInfo(step);

    const defaultValues = useMemo(() => {
        if (defaultData) return defaultData;

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
            return finalSchema.partial({ clientSecret: true });

        return SchemaMap[step];
    };

    const form = useForm<FormData>({
        resolver: zodResolver(getStepSchema(step)),
        defaultValues,
    });

    useEffect(() => {
        form.reset(defaultValues);
    }, [defaultValues, form]);

    const handleSubmit = form.handleSubmit(async (data) => {
        if (step === Step.NAME) {
            // try creating the app if there is no existing app
            if (!isEdit) {
                const result = await createApp.executeAsync({
                    type: OAuthProvider.Custom,
                    global: true,
                    ...data,
                });

                if (result.error) {
                    toast.error("Failed to create OAuth app");
                    form.setError("integration", {
                        message: "Integration name already taken",
                    });

                    // do not proceed to the next step if there's an error
                    return;
                }
            }
        }

        if (!isFinal) {
            onNext();
            return;
        }

        if (!app) {
            // should never happen
            // indicates that step 1 was not completed
            throw new Error("App is required");
        }

        updateApp.execute(app.id, { ...data });
    });

    // once a user touches the integration field, we don't auto-derive it from the name
    const deriveIntegrationFromName =
        !initialIsEdit && !form.formState.touchedFields.integration;

    return (
        <Form {...form}>
            <form onSubmit={handleSubmit} className="space-y-4">
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
                    </>
                )}

                {step === Step.INFO && (
                    <>
                        <CopyText
                            text={app!.links.redirectURL}
                            label="Redirect URL"
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
                            placeholder={
                                initialIsEdit ? "(Unchanged)" : undefined
                            }
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

                <div className="flex gap-2">
                    <Button
                        className="flex-1 w-full"
                        type="button"
                        variant="outline"
                        onClick={onBack}
                    >
                        {prevLabel}
                    </Button>

                    <Button className="flex-1 w-full" type="submit">
                        {nextLabel}
                    </Button>
                </div>
            </form>
        </Form>
    );

    function getStepInfo(step: Step) {
        if (step === Step.INFO) {
            return {
                isFinal: true,
                nextLabel: "Submit",
                prevLabel: "Back",
                onBack: () => setStep((prev) => (prev - 1) as Step),
            } as const;
        }

        return {
            nextLabel: "Next",
            prevLabel: "Cancel",
            onBack: onCancel,
            onNext: () => {
                return setStep((prev) => (prev + 1) as Step);
            },
        } as const;
    }
}

function convertToIntegration(name: string) {
    return name.toLowerCase().replace(/[\s\W]+/g, "-");
}
