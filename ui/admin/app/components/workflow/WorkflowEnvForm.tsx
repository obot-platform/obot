import { Workflow } from "~/lib/model/workflows";

import { NameDescriptionForm } from "~/components/workflow/NameDescriptionForm";

type WorkflowEnvFormProps = {
    workflow: Workflow;
    onChange?: (values: { env: Workflow["env"] }) => void;
};

export function WorkflowEnvForm({ workflow, onChange }: WorkflowEnvFormProps) {
    return (
        <NameDescriptionForm
            addLabel="Add Environment Variable"
            defaultValues={
                workflow.env?.length
                    ? workflow.env
                    : [{ name: "", description: "" }]
            }
            onChange={(values) =>
                onChange?.({
                    env: values.map((item) => ({ ...item, value: "" })),
                })
            }
        />
    );
}
