import { CheckIcon, ChevronsUpDownIcon } from "lucide-react";
import { ReactNode, useState } from "react";

import { Button } from "~/components/ui/button";
import {
    Command,
    CommandEmpty,
    CommandGroup,
    CommandInput,
    CommandItem,
    CommandList,
} from "~/components/ui/command";
import { Drawer, DrawerContent, DrawerTrigger } from "~/components/ui/drawer";
import {
    Popover,
    PopoverContent,
    PopoverTrigger,
} from "~/components/ui/popover";
import { useIsMobile } from "~/hooks/use-mobile";

type BaseOption = {
    id: string;
    name?: string | undefined;
};

type GroupedOption<T extends BaseOption> = {
    heading: string;
    value: T[];
};

type ComboBoxProps<T extends BaseOption> = {
    allowClear?: boolean;
    clearLabel?: ReactNode;
    onChange: (option: T | null) => void;
    options: T[] | GroupedOption<T>[];
    placeholder?: string;
    value?: T | null;
};

export function ComboBox<T extends BaseOption>({
    disabled,
    placeholder,
    value,
    ...props
}: {
    disabled?: boolean;
} & ComboBoxProps<T>) {
    const [open, setOpen] = useState(false);
    const isMobile = useIsMobile();

    if (!isMobile) {
        return (
            <Popover open={open} onOpenChange={setOpen}>
                <PopoverTrigger asChild>{renderButtonContent()}</PopoverTrigger>
                <PopoverContent className="w-full p-0" align="start">
                    <ComboBoxList setOpen={setOpen} value={value} {...props} />
                </PopoverContent>
            </Popover>
        );
    }

    return (
        <Drawer open={open} onOpenChange={setOpen}>
            <DrawerTrigger asChild>{renderButtonContent()}</DrawerTrigger>
            <DrawerContent>
                <div className="mt-4 border-t">
                    <ComboBoxList setOpen={setOpen} value={value} {...props} />
                </div>
            </DrawerContent>
        </Drawer>
    );

    function renderButtonContent() {
        return (
            <Button
                disabled={disabled}
                endContent={<ChevronsUpDownIcon />}
                variant="outline"
                className="px-3 w-full font-normal justify-start rounded-sm"
                classNames={{
                    content: "w-full justify-between",
                }}
            >
                <span className="text-ellipsis overflow-hidden">
                    {value ? value.name : placeholder}
                </span>
            </Button>
        );
    }
}

function ComboBoxList<T extends BaseOption>({
    allowClear,
    clearLabel,
    onChange,
    options,
    placeholder = "Filter...",
    setOpen,
    value,
}: { setOpen: (open: boolean) => void } & ComboBoxProps<T>) {
    const isGrouped = options.every((option) => "heading" in option);
    return (
        <Command>
            <CommandInput placeholder={placeholder} />
            <CommandList>
                <CommandEmpty>No results found.</CommandEmpty>
                {allowClear && (
                    <CommandGroup>
                        <CommandItem
                            onSelect={() => {
                                onChange(null);
                                setOpen(false);
                            }}
                        >
                            {clearLabel ?? "Clear Selection"}
                        </CommandItem>
                    </CommandGroup>
                )}
                {isGrouped
                    ? renderGroupedOptions(options)
                    : renderUngroupedOptions(options)}
            </CommandList>
        </Command>
    );

    function renderGroupedOptions(items: GroupedOption<T>[]) {
        return items.map((group) => (
            <CommandGroup key={group.heading} heading={group.heading}>
                {group.value.map((option) => (
                    <CommandItem
                        key={option.id}
                        value={option.name}
                        onSelect={(name) => {
                            const match =
                                group.value.find((opt) => opt.name === name) ||
                                null;
                            onChange(match);
                            setOpen(false);
                        }}
                        className="justify-between"
                    >
                        {option.name || option.id}{" "}
                        {value?.id === option.id && (
                            <CheckIcon className="w-4 h-4" />
                        )}
                    </CommandItem>
                ))}
            </CommandGroup>
        ));
    }

    function renderUngroupedOptions(items: T[]) {
        return (
            <CommandGroup>
                {items.map((option) => (
                    <CommandItem
                        key={option.id}
                        value={option.name}
                        onSelect={(name) => {
                            const match =
                                items.find((opt) => opt.name === name) || null;
                            onChange(match);
                            setOpen(false);
                        }}
                        className="justify-between"
                    >
                        {option.name || option.id}{" "}
                        {value?.id === option.id && (
                            <CheckIcon className="w-4 h-4" />
                        )}
                    </CommandItem>
                ))}
            </CommandGroup>
        );
    }
}
