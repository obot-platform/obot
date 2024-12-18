import { useState } from "react";

import { RevealedEnv } from "~/lib/model/environmentVariables";

import { NameDescriptionForm } from "~/components/composed/NameDescriptionForm";
import { Button } from "~/components/ui/button";

type EnvFormProps = {
    defaultValues: RevealedEnv;
    onSubmit: (values: RevealedEnv) => void;
    isLoading: boolean;
};

export function EnvForm({
    defaultValues,
    onSubmit: updateEnv,
    isLoading,
}: EnvFormProps) {
    const [state, setState] = useState(() =>
        Object.entries(defaultValues).map(([name, description]) => ({
            name,
            description,
        }))
    );

    const handleSubmit = (e: React.FormEvent<HTMLFormElement>) => {
        e.preventDefault();

        if (defaultValues) {
            const updates = Object.fromEntries(
                state.map(({ name, description }) => [name, description])
            );

            updateEnv(updates);
        }
    };

    return (
        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
            <NameDescriptionForm
                defaultValues={state}
                onChange={setState}
                descriptionFieldProps={{
                    type: "password",
                    placeholder: "Value",
                }}
            />

            <Button className="w-full" type="submit" loading={isLoading}>
                Save
            </Button>
        </form>
    );
}
