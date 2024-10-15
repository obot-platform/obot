import { zodResolver } from "@hookform/resolvers/zod";
import { useMemo } from "react";
import { useForm } from "react-hook-form";
import { z } from "zod";

import {
    OAuthAppParams,
    OAuthAppSpec,
    OAuthAppType,
} from "~/lib/model/oauthApps";

import { ControlledInput } from "~/components/form/controlledInputs";
import { Button } from "~/components/ui/button";
import { Form } from "~/components/ui/form";

type OAuthAppFormProps = {
    appSpec: OAuthAppSpec[OAuthAppType];
    onSubmit: (data: OAuthAppParams) => void;
};

export function OAuthAppForm({ appSpec, onSubmit }: OAuthAppFormProps) {
    const fields = Object.entries(appSpec.parameters).map(([key, label]) => ({
        key: key as keyof OAuthAppParams,
        label,
    }));

    const schema = useMemo(() => {
        return z.object(
            Object.entries(appSpec.parameters).reduce(
                (acc, [key]) => {
                    acc[key as keyof OAuthAppParams] = z.string();
                    return acc;
                },
                {} as Record<keyof OAuthAppParams, z.ZodType>
            )
        );
    }, [appSpec.parameters]);

    const form = useForm({
        defaultValues: Object.entries(appSpec.parameters).reduce(
            (acc, [key]) => {
                acc[key as keyof OAuthAppParams] = "";
                return acc;
            },
            {} as OAuthAppParams
        ),
        resolver: zodResolver(schema),
    });

    const handleSubmit = form.handleSubmit(onSubmit);

    return (
        <Form {...form}>
            <form onSubmit={handleSubmit} className="flex flex-col gap-4">
                {fields.map(({ key, label }) => (
                    <ControlledInput
                        key={key}
                        name={key}
                        label={label}
                        control={form.control}
                    />
                ))}

                <Button type="submit">Submit</Button>
            </form>
        </Form>
    );
}
