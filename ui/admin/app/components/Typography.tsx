import React, { ReactNode } from "react";

import { cn } from "~/lib/utils";

type TypographyElement = keyof JSX.IntrinsicElements;

type TypographyProps<T extends TypographyElement> = {
    children: ReactNode;
    className?: string;
} & React.JSX.IntrinsicElements[T];

export function TypographyH1({
    children,
    className,
    ...props
}: TypographyProps<"h1">) {
    return (
        <h1
            className={cn(
                `scroll-m-20 text-4xl font-extrabold tracking-tight lg:text-5xl`,
                className
            )}
            {...props}
        >
            {children}
        </h1>
    );
}

export function TypographyH2({
    children,
    className,
    ...props
}: TypographyProps<"h2">) {
    return (
        <h2
            className={cn(
                `scroll-m-20 pb-2 text-3xl font-semibold tracking-tight first:mt-0`,
                className
            )}
            {...props}
        >
            {children}
        </h2>
    );
}

export function TypographyH3({
    children,
    className,
    ...props
}: TypographyProps<"h3">) {
    return (
        <h3
            className={cn(
                `scroll-m-20 text-2xl font-semibold tracking-tight`,
                className
            )}
            {...props}
        >
            {children}
        </h3>
    );
}

export function TypographyH4({
    children,
    className,
    ...props
}: TypographyProps<"h4">) {
    return (
        <h4
            className={cn(
                `scroll-m-20 text-xl font-semibold tracking-tight`,
                className
            )}
            {...props}
        >
            {children}
        </h4>
    );
}

export function TypographyP({
    children,
    className,
    ...props
}: TypographyProps<"p">) {
    return (
        <p className={cn(`leading-7`, className)} {...props}>
            {children}
        </p>
    );
}

export function TypographyPAccent({
    children,
    className,
    ...props
}: TypographyProps<"p">) {
    return (
        <TypographyP className={cn(`text-secondary`, className)} {...props}>
            {children}
        </TypographyP>
    );
}

export function TypographyBlockquote({
    children,
    className,
    ...props
}: TypographyProps<"blockquote">) {
    return (
        <blockquote
            className={cn(`mt-6 border-l-2 pl-6 italic`, className)}
            {...props}
        >
            {children}
        </blockquote>
    );
}

export function TypographyInlineCode({
    children,
    className,
    ...props
}: TypographyProps<"code">) {
    return (
        <code
            className={cn(
                `relative rounded bg-muted px-[0.3rem] py-[0.2rem] font-mono text-sm font-semibold`,
                className
            )}
            {...props}
        >
            {children}
        </code>
    );
}

export function TypographyLead({
    children,
    className,
    ...props
}: TypographyProps<"p">) {
    return (
        <p
            className={cn(`text-xl text-muted-foreground`, className)}
            {...props}
        >
            {children}
        </p>
    );
}

export function TypographyLarge({
    children,
    className,
    ...props
}: TypographyProps<"div">) {
    return (
        <div className={cn(`text-lg font-semibold`, className)} {...props}>
            {children}
        </div>
    );
}

export function TypographySmall({
    children,
    className,
    ...props
}: TypographyProps<"small">) {
    return (
        <small
            className={cn(`text-sm font-medium leading-none`, className)}
            {...props}
        >
            {children}
        </small>
    );
}

export function TypographyMuted({
    children,
    className,
    ...props
}: TypographyProps<"p">) {
    return (
        <p
            className={cn(`text-sm text-muted-foreground`, className)}
            {...props}
        >
            {children}
        </p>
    );
}

export function TypographyMutedAccent({
    children,
    className,
    ...props
}: TypographyProps<"p">) {
    return (
        <p className={cn(`text-sm text-blue-500`, className)} {...props}>
            {children}
        </p>
    );
}
