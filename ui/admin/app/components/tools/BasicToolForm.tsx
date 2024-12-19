import { memo, useCallback, useMemo, useState } from "react";

import { ToolEntry } from "~/components/agent/ToolEntry";
import { ToolCatalogDialog } from "~/components/tools/ToolCatalog";

export const BasicToolForm = memo(function BasicToolFormComponent(props: {
    value?: string[];
    defaultValue?: string[];
    onChange?: (values: string[]) => void;
}) {
    const { onChange } = props;

    const [_value, _setValue] = useState(props.defaultValue);
    const value = useMemo(
        () => props.value ?? _value ?? [],
        [props.value, _value]
    );

    const setValue = useCallback(
        (newValue: string[]) => {
            _setValue(newValue);
            onChange?.(newValue);
        },
        [onChange]
    );

    const removeTools = (toolsToRemove: string[]) => {
        setValue(value.filter((tool) => !toolsToRemove.includes(tool)));
    };

    const addTool = (tool: string) => {
        setValue([...value, tool]);
    };

    return (
        <div className="flex flex-col gap-2">
            <div className="flex flex-col gap-1 w-full overflow-y-auto">
                {value.map((tool) => (
                    <ToolEntry
                        key={tool}
                        tool={tool}
                        onDelete={() => removeTools([tool])}
                    />
                ))}
            </div>

            <div className="flex justify-end">
                <ToolCatalogDialog
                    tools={value}
                    onAddTool={addTool}
                    onRemoveTools={removeTools}
                    onUpdateTools={setValue}
                />
            </div>
        </div>
    );
});
