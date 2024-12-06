import { memo, useState } from "react";

import { Model, ModelUsage, getModelUsageLabel } from "~/lib/model/models";
import { ModelApiService } from "~/lib/service/api/modelApiService";

import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from "~/components/ui/select";

export const UpdateModelUsage = memo(function UpdateModelUsage({
    model,
    onChange,
}: {
    model: Model;
    onChange?: (usage: ModelUsage) => void;
}) {
    const [usage, setUsage] = useState(model.usage);
    const handleModelUsageChange = (value: string) => {
        const updatedUsage = value as ModelUsage;
        ModelApiService.updateModel(model.id, {
            ...model,
            usage: updatedUsage,
        });
        setUsage(updatedUsage);
        onChange?.(updatedUsage);
    };

    return (
        <Select onValueChange={handleModelUsageChange} value={usage}>
            <SelectTrigger>
                <SelectValue placeholder="Select Usage..." />
            </SelectTrigger>

            <SelectContent>
                {Object.entries(ModelUsage).map(([key, value]) =>
                    value === ModelUsage.Unknown ? null : (
                        <SelectItem key={key} value={value}>
                            {getModelUsageLabel(value)}
                        </SelectItem>
                    )
                )}
            </SelectContent>
        </Select>
    );
});
