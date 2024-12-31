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
    value: (T | GroupedOption<T>)[];
};

type ComboBoxProps<T extends BaseOption> = {
    allowClear?: boolean;
    clearLabel?: ReactNode;
    emptyLabel?: ReactNode;
    onChange: (option: T | null) => void;
    options: (T | GroupedOption<T>)[];
    placeholder?: string;
    renderOption?: (option: T) => ReactNode;
    value?: T | null;
};

export function ComboBox<T extends BaseOption>({
    disabled,
    placeholder,
    value,
    renderOption,
    ...props
}: {
    disabled?: boolean;
} & ComboBoxProps<T>) {
    const [open, setOpen] = useState(false);
    const isMobile = useIsMobile();

    if (!isMobile) {
        return (
            <Popover modal={true} open={open} onOpenChange={setOpen}>
                <PopoverTrigger asChild>{renderButtonContent()}</PopoverTrigger>
                <PopoverContent className="w-full p-0" align="start">
                    <ComboBoxList
                        setOpen={setOpen}
                        renderOption={renderOption}
                        value={value}
                        {...props}
                    />
                </PopoverContent>
            </Popover>
        );
    }

    return (
        <Drawer open={open} onOpenChange={setOpen}>
            <DrawerTrigger asChild>{renderButtonContent()}</DrawerTrigger>
            <DrawerContent>
                <div className="mt-4 border-t">
                    <ComboBoxList
                        setOpen={setOpen}
                        renderOption={renderOption}
                        value={value}
                        {...props}
                    />
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
                    {renderOption && value
                        ? renderOption(value)
                        : (value?.name ?? placeholder)}
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
    setOpen,
    renderOption,
    value,
    placeholder = "Filter...",
    emptyLabel = "No results found.",
}: { setOpen: (open: boolean) => void } & ComboBoxProps<T>) {
    const [filteredOptions, setFilteredOptions] =
        useState<typeof options>(options);

    const filterOptions = (
        items: (T | GroupedOption<T>)[],
        searchValue: string
    ): (T | GroupedOption<T>)[] =>
        items.reduce(
            (acc, option) => {
                const isSingleValueMatch =
                    "name" in option &&
                    option.name
                        ?.toLowerCase()
                        .includes(searchValue.toLowerCase());
                const isGroupHeadingMatch =
                    "heading" in option &&
                    option.heading
                        .toLowerCase()
                        .includes(searchValue.toLowerCase());

                if (isGroupHeadingMatch || isSingleValueMatch) {
                    return [...acc, option];
                }

                if ("heading" in option) {
                    const matches = filterOptions(option.value, searchValue);
                    return matches.length > 0
                        ? [
                              ...acc,
                              {
                                  ...option,
                                  value: matches,
                              },
                          ]
                        : acc;
                }

                return acc;
            },
            [] as (T | GroupedOption<T>)[]
        );

    const handleValueChange = (value: string) => {
        setFilteredOptions(filterOptions(options, value));
    };

    return (
        <Command
            shouldFilter={false}
            className="w-[var(--radix-popper-anchor-width)] max-h-[50vh]"
        >
            <CommandInput
                placeholder={placeholder}
                onValueChange={handleValueChange}
            />
            <CommandList>
                <CommandEmpty>{emptyLabel}</CommandEmpty>
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
                {filteredOptions.map((option) =>
                    "heading" in option
                        ? renderGroupedOption(option)
                        : renderUngroupedOption(option)
                )}
            </CommandList>
        </Command>
    );

    function renderGroupedOption(group: GroupedOption<T>) {
        return (
            <CommandGroup key={group.heading} heading={group.heading}>
                {group.value.map((option) =>
                    "heading" in option
                        ? renderGroupedOption(option)
                        : renderUngroupedOption(option)
                )}
            </CommandGroup>
        );
    }

    function renderUngroupedOption(option: T) {
        return (
            <CommandItem
                key={option.id}
                value={option.name}
                onSelect={() => {
                    onChange(option);
                    setOpen(false);
                }}
                className="justify-between"
            >
                <span>
                    {renderOption
                        ? renderOption(option)
                        : (option.name ?? option.id)}
                </span>
                {value?.id === option.id && <CheckIcon className="w-4 h-4" />}
            </CommandItem>
        );
    }
}
