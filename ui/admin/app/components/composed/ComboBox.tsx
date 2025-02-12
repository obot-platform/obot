import { CheckIcon, ChevronsUpDownIcon } from "lucide-react";
import { ReactNode, useState } from "react";

import { cn } from "~/lib/utils";

import { Button, ButtonProps } from "~/components/ui/button";
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
	name?: string;
	sublabel?: string;
};

type GroupedOption<T extends BaseOption> = {
	heading: string;
	value: (T | GroupedOption<T>)[];
};

type ComboBoxProps<T extends BaseOption> = {
	allowClear?: boolean;
	allowCreate?: boolean;
	buttonProps?: ButtonProps;
	clearLabel?: ReactNode;
	emptyLabel?: ReactNode;
	onChange: (option: T | null) => void;
	onCreate?: (value: string) => void;
	options: (T | GroupedOption<T>)[];
	closeOnSelect?: boolean;
	placeholder?: string;
	renderOption?: (option: T) => ReactNode;
	validateCreate?: (value: string) => boolean;
	value?: T | null;
	classNames?: {
		command?: string;
	};
};

export function ComboBox<T extends BaseOption>({
	buttonProps,
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
				className="w-full justify-start rounded-sm px-3 font-normal"
				classNames={{
					content: "w-full justify-between",
				}}
				{...buttonProps}
			>
				<span className="overflow-hidden text-ellipsis">
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
	allowCreate,
	clearLabel,
	onChange,
	onCreate,
	options,
	setOpen,
	renderOption,
	validateCreate,
	value,
	placeholder = "Filter...",
	emptyLabel = "No results found.",
	closeOnSelect = true,
	classNames,
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
					option.name?.toLowerCase().includes(searchValue.toLowerCase());
				const isGroupHeadingMatch =
					"heading" in option &&
					option.heading.toLowerCase().includes(searchValue.toLowerCase());

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
		setSavedValue(value);
		setFilteredOptions(filterOptions(options, value));
	};

	const [savedValue, setSavedValue] = useState("");
	return (
		<Command
			shouldFilter={false}
			className={cn(
				"max-h-[50vh] w-[var(--radix-popper-anchor-width)]",
				classNames?.command
			)}
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
								if (closeOnSelect) setOpen(false);
							}}
						>
							{clearLabel ?? "Clear Selection"}
						</CommandItem>
					</CommandGroup>
				)}
				{allowCreate && savedValue.length > 0 && (
					<CommandGroup>
						<CommandItem
							onSelect={() => {
								onCreate?.(savedValue);
								setOpen(false);
							}}
							disabled={validateCreate ? !validateCreate(savedValue) : false}
						>
							Add &quot;{savedValue}&quot;
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
			<CommandGroup
				key={group.heading}
				heading={group.heading}
				className="[&_[cmdk-group-heading]]:px-1 [&_[cmdk-group-heading]]:text-sm [&_[cmdk-group-heading]]:font-bold"
			>
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
				value={option.id}
				onSelect={() => {
					onChange(option);
					if (closeOnSelect) setOpen(false);
				}}
				className="justify-between"
			>
				<span>
					{renderOption ? renderOption(option) : (option.name ?? option.id)}
					{option.sublabel && (
						<span className="text-xs text-muted-foreground">
							{" "}
							({option.sublabel})
						</span>
					)}
				</span>
				{value?.id === option.id && <CheckIcon className="h-4 w-4" />}
			</CommandItem>
		);
	}
}
