import { ComponentProps, useMemo } from "react";

import { Workflow } from "~/lib/model/workflows";

import { NameDescriptionForm } from "~/components/workflow/NameDescriptionForm";

type ParamValues = Workflow["params"];

const convertFrom = (params: ParamValues) => {
    const converted = Object.entries(params || {}).map(
        ([name, description]) => ({
            name,
            description,
        })
    );

    return converted.length ? converted : [{ name: "", description: "" }];
};

const convertTo = (
    params: ComponentProps<typeof NameDescriptionForm>["defaultValues"]
) => {
    if (!params?.length) return undefined;

    return params.reduce((acc, param) => {
        if (!param.name) return acc;

        acc[param.name] = param.description;
        return acc;
    }, {} as NonNullable<ParamValues>);
};

export function ParamsForm({
    workflow,
    onChange,
}: {
    workflow: Workflow;
    onChange?: (values: { params?: ParamValues }) => void;
}) {
    const defaultValues = useMemo(
        () => convertFrom(workflow.params),
        [workflow.params]
    );

    return (
        <NameDescriptionForm
            addLabel="Add Parameter"
            defaultValues={defaultValues}
            onChange={(values) => onChange?.({ params: convertTo(values) })}
        />
    );
}
