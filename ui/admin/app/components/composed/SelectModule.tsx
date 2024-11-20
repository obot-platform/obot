import { PlusIcon, TrashIcon } from "lucide-react";
import { useMemo } from "react";

import { Button } from "~/components/ui/button";
import {
    Command,
    CommandEmpty,
    CommandInput,
    CommandItem,
    CommandList,
} from "~/components/ui/command";
import {
    Popover,
    PopoverContent,
    PopoverTrigger,
} from "~/components/ui/popover";

interface SelectModuleProps<T> {
    items?: T[];
    selection: string[];
    onChange: (selection: string[]) => void;
    renderDropdownItem: (item: T) => React.ReactNode;
    renderListItem: (item: T) => React.ReactNode;
    buttonText?: string;
    searchPlaceholder?: string;
    emptyMessage?: string;
    getItemKey: (item: T) => string;
}

export function SelectModule<T>({
    items = [],
    selection,
    onChange,
    renderDropdownItem,
    renderListItem,
    buttonText,
    searchPlaceholder,
    emptyMessage,
    getItemKey,
}: SelectModuleProps<T>) {
    return (
        <div className="flex flex-col gap-2">
            <SelectList
                selected={selection}
                items={items}
                onRemove={(id) => onChange(selection.filter((s) => s !== id))}
                renderItem={renderListItem}
                getItemKey={getItemKey}
            />

            <SelectPopover
                className="self-end"
                items={items}
                onSelect={(item) => onChange([...selection, getItemKey(item)])}
                filter={(item) => !selection.includes(getItemKey(item))}
                renderItem={renderDropdownItem}
                buttonText={buttonText}
                searchPlaceholder={searchPlaceholder}
                emptyMessage={emptyMessage}
                getItemKey={getItemKey}
            />
        </div>
    );
}

interface SelectProps<T> {
    items?: T[];
    onSelect: (item: T) => void;
    filter?: (item: T, index: number, array: T[]) => boolean;
    renderItem: (item: T) => React.ReactNode;
    searchPlaceholder?: string;
    emptyMessage?: string;
    getItemKey: (item: T) => string;
}

export function Select<T>({
    items = [],
    onSelect,
    filter,
    renderItem,
    searchPlaceholder = "Search...",
    emptyMessage = "No items to select",
    getItemKey,
}: SelectProps<T>) {
    const filteredItems = filter ? items.filter(filter) : items;

    return (
        <Command>
            <CommandInput placeholder={searchPlaceholder} />
            <CommandList>
                {filteredItems?.length ? (
                    filteredItems?.map((item) => (
                        <CommandItem
                            key={getItemKey(item)}
                            value={getItemKey(item)}
                            onSelect={() => onSelect(item)}
                        >
                            {renderItem(item)}
                        </CommandItem>
                    ))
                ) : (
                    <CommandEmpty>{emptyMessage}</CommandEmpty>
                )}
            </CommandList>
        </Command>
    );
}

interface SelectPopoverProps<T> extends SelectProps<T> {
    className?: string;
    buttonText?: string;
}

export function SelectPopover<T>({
    className,
    buttonText = "Select Item",
    ...props
}: SelectPopoverProps<T>) {
    return (
        <Popover>
            <PopoverTrigger asChild>
                <Button
                    variant="secondary"
                    startContent={<PlusIcon />}
                    className={className}
                >
                    {buttonText}
                </Button>
            </PopoverTrigger>

            <PopoverContent className="p-0" align="end">
                <Select {...props} />
            </PopoverContent>
        </Popover>
    );
}

interface SelectListProps<T> {
    selected: string[];
    items?: T[];
    onRemove: (id: string) => void;
    renderItem: (item: T) => React.ReactNode;
    fallbackRender?: (id: string) => React.ReactNode;
    getItemKey: (item: T) => string;
}

export function SelectList<T>({
    selected,
    items = [],
    onRemove,
    renderItem,
    fallbackRender = (id) => id,
    getItemKey,
}: SelectListProps<T>) {
    const itemMap = useMemo(() => {
        return items.reduce(
            (acc, item) => {
                acc[getItemKey(item)] = item;
                return acc;
            },
            {} as Record<string, T>
        );
    }, [items, getItemKey]);

    return (
        <div className="flex flex-col gap-2 divide-y">
            {selected.map((id) => (
                <div
                    key={id}
                    className="flex items-center justify-between gap-2 pt-2"
                >
                    {itemMap[id] ? renderItem(itemMap[id]) : fallbackRender(id)}

                    <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => onRemove(id)}
                    >
                        <TrashIcon />
                    </Button>
                </div>
            ))}
        </div>
    );
}
