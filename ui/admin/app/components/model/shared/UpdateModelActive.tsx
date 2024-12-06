import { memo, useCallback, useState } from "react";

import { Model } from "~/lib/model/models";
import { ModelApiService } from "~/lib/service/api/modelApiService";

import { Switch } from "~/components/ui/switch";

export const UpdateModelActive = memo(function UpdateModelActive({
    model,
    onChange,
}: {
    model: Model;
    onChange?: (active: boolean) => void;
}) {
    const [active, setActive] = useState(model.active);
    const handleModelStatusChange = useCallback(
        (checked: boolean) => {
            ModelApiService.updateModel(model.id, {
                ...model,
                active: checked,
            });
            setActive(checked);
            onChange?.(checked);
        },
        [model, onChange]
    );

    return (
        <Switch checked={active} onCheckedChange={handleModelStatusChange} />
    );
});
