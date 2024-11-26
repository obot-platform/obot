import { ComponentProps, ReactNode } from "react";
import {
    Control,
    ControllerFieldState,
    ControllerRenderProps,
    FieldPath,
    FieldValues,
    FormState,
} from "react-hook-form";

import { cn } from "~/lib/utils";

import {
    BasicInputItem,
    BasicInputItemProps,
} from "~/components/form/BasicInputItem";
import { Checkbox } from "~/components/ui/checkbox";
import {
    FormControl,
    FormDescription,
    FormField,
    FormItem,
    FormLabel,
    FormMessage,
} from "~/components/ui/form";
import { Input, InputProps } from "~/components/ui/input";
import {
    AutosizeTextAreaProps,
    AutosizeTextarea,
    Textarea,
    TextareaProps,
} from "~/components/ui/textarea";

type BaseProps<
    TValues extends FieldValues,
    TName extends FieldPath<TValues>,
> = {
    control: Control<TValues>;
    name: TName;
    label?: ReactNode;
    description?: ReactNode;
};

export type ControlledInputProps<
    TValues extends FieldValues,
    TName extends FieldPath<TValues>,
> = InputProps &
    BaseProps<TValues, TName> & {
        onChangeConversion?: (value: string) => string;
        classNames?: {
            wrapper?: string;
            label?: string;
            input?: string;
            description?: string;
            message?: string;
        };
    };

export function ControlledInput<
    TValues extends FieldValues,
    TName extends FieldPath<TValues>,
>({
    control,
    name,
    label,
    className,
    description,
    onChange,
    onChangeConversion,
    classNames = {},
    ...inputProps
}: ControlledInputProps<TValues, TName>) {
    return (
        <FormField
            control={control}
            name={name}
            render={({ field, fieldState }) => (
                <BasicInputItem
                    classNames={classNames}
                    label={label}
                    description={description}
                >
                    <Input
                        {...field}
                        {...inputProps}
                        onChange={(e) => {
                            if (onChangeConversion) {
                                e.target.value = onChangeConversion(
                                    e.target.value
                                );
                            }

                            field.onChange(e);
                            onChange?.(e);
                        }}
                        className={cn(
                            getFieldStateClasses(fieldState),
                            className
                        )}
                    />
                </BasicInputItem>
            )}
        />
    );
}

export type ControlledTextareaProps<
    TValues extends FieldValues,
    TName extends FieldPath<TValues>,
> = Omit<TextareaProps, keyof ControllerRenderProps<TValues, TName>> &
    BaseProps<TValues, TName>;

export function ControlledTextarea<
    TValues extends FieldValues,
    TName extends FieldPath<TValues>,
>({
    control,
    name,
    label,
    description,
    className,
    ...inputProps
}: ControlledTextareaProps<TValues, TName>) {
    return (
        <FormField
            control={control}
            name={name}
            render={({ field, fieldState }) => (
                <BasicInputItem label={label} description={description}>
                    <Textarea
                        {...field}
                        {...inputProps}
                        className={cn(
                            getFieldStateClasses(fieldState),
                            className
                        )}
                    />
                </BasicInputItem>
            )}
        />
    );
}

export type ControlledAutosizeTextareaProps<
    TValues extends FieldValues,
    TName extends FieldPath<TValues>,
> = Omit<AutosizeTextAreaProps, keyof ControllerRenderProps<TValues, TName>> &
    BaseProps<TValues, TName>;

export function ControlledAutosizeTextarea<
    TValues extends FieldValues,
    TName extends FieldPath<TValues>,
>({
    control,
    name,
    label,
    description,
    className,
    ...inputProps
}: ControlledAutosizeTextareaProps<TValues, TName>) {
    return (
        <FormField
            control={control}
            name={name}
            render={({ field, fieldState }) => (
                <BasicInputItem label={label} description={description}>
                    <AutosizeTextarea
                        {...field}
                        {...inputProps}
                        className={cn(
                            getFieldStateClasses(fieldState),
                            className
                        )}
                    />
                </BasicInputItem>
            )}
        />
    );
}

export type ControlledCheckboxProps<
    TValues extends FieldValues,
    TName extends FieldPath<TValues>,
> = BaseProps<TValues, TName> & ComponentProps<typeof Checkbox>;

export function ControlledCheckbox<
    TValues extends FieldValues,
    TName extends FieldPath<TValues>,
>({
    control,
    name,
    label,
    description,
    onCheckedChange,
    ...checkboxProps
}: ControlledCheckboxProps<TValues, TName>) {
    return (
        <FormField
            control={control}
            name={name}
            render={({ field, fieldState }) => (
                <FormItem>
                    <div className="flex items-center gap-2">
                        <FormControl>
                            <Checkbox
                                {...field}
                                {...checkboxProps}
                                checked={field.value}
                                onCheckedChange={(value) => {
                                    field.onChange(value);
                                    onCheckedChange?.(value);
                                }}
                                className={cn(
                                    getFieldStateClasses(fieldState),
                                    checkboxProps.className
                                )}
                            />
                        </FormControl>

                        {label && <FormLabel>{label}</FormLabel>}
                    </div>

                    <FormMessage />

                    {description && (
                        <FormDescription>{description}</FormDescription>
                    )}
                </FormItem>
            )}
        />
    );
}

export type ControlledCustomInputProps<
    TValues extends FieldValues,
    TName extends FieldPath<TValues>,
> = BaseProps<TValues, TName> & {
    classNames?: BasicInputItemProps["classNames"];
    children: (props: {
        field: ControllerRenderProps<TValues, TName>;
        fieldState: ControllerFieldState;
        formState: FormState<TValues>;
        className?: string;
    }) => ReactNode;
};

export function ControlledCustomInput<
    TValues extends FieldValues,
    TName extends FieldPath<TValues>,
>({
    control,
    name,
    label,
    description,
    classNames,
    children,
}: ControlledCustomInputProps<TValues, TName>) {
    return (
        <FormField
            control={control}
            name={name}
            render={(args) => (
                <BasicInputItem
                    classNames={classNames}
                    label={label}
                    description={description}
                >
                    {children({
                        ...args,
                        className: getFieldStateClasses(args.fieldState),
                    })}
                </BasicInputItem>
            )}
        />
    );
}

function getFieldStateClasses(fieldState: ControllerFieldState) {
    return cn({
        "focus-visible:ring-destructive border-destructive": fieldState.invalid,
    });
}
