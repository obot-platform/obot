import { ReactNode } from "react";
import {
    Control,
    ControllerFieldState,
    ControllerRenderProps,
    FieldPath,
    FieldValues,
} from "react-hook-form";

import { cn } from "~/lib/utils";

import {
    FormControl,
    FormDescription,
    FormField,
    FormItem,
    FormLabel,
    FormMessage,
} from "~/components/ui/form";
import { Input, InputProps } from "~/components/ui/input";
import { Textarea, TextareaProps } from "~/components/ui/textarea";

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
> = Omit<InputProps, keyof ControllerRenderProps<TValues, TName>> &
    BaseProps<TValues, TName>;

export function ControlledInput<
    TValues extends FieldValues,
    TName extends FieldPath<TValues>,
>({
    control,
    name,
    label,
    className,
    description,
    ...inputProps
}: ControlledInputProps<TValues, TName>) {
    return (
        <FormField
            control={control}
            name={name}
            render={({ field, fieldState }) => (
                <FormItem>
                    {label && <FormLabel>{label}</FormLabel>}

                    {description && (
                        <FormDescription hidden>{description}</FormDescription>
                    )}

                    <FormControl>
                        <Input
                            {...field}
                            {...inputProps}
                            className={cn(
                                getFieldStateClasses(fieldState),
                                className
                            )}
                        />
                    </FormControl>

                    <FormMessage />
                </FormItem>
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
                <FormItem>
                    {label && <FormLabel>{label}</FormLabel>}

                    {description && (
                        <FormDescription hidden>{description}</FormDescription>
                    )}

                    <FormControl>
                        <Textarea
                            {...field}
                            {...inputProps}
                            className={cn(
                                getFieldStateClasses(fieldState),
                                className
                            )}
                        />
                    </FormControl>

                    <FormMessage />
                </FormItem>
            )}
        />
    );
}

function getFieldStateClasses(fieldState: ControllerFieldState) {
    return cn({
        "focus-visible:ring-destructive border-destructive": fieldState.invalid,
    });
}
