import { Slot } from "@radix-ui/react-slot";
import { type VariantProps, cva } from "class-variance-authority";
import { Loader2 } from "lucide-react";
import * as React from "react";

import { cn } from "~/lib/utils";

export const ButtonClasses = {
    base: "inline-flex items-center justify-center gap-2 whitespace-nowrap text-sm font-medium transition-colors focus-visible:outline-none hover:shadow-inner focus-visible:ring-1 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0",
    variants: {
        variant: {
            default:
                "bg-primary text-primary-foreground shadow hover:bg-primary/80",
            destructive:
                "bg-destructive text-destructive-foreground shadow-sm hover:bg-destructive/80",
            outline:
                "border border-input bg-background shadow-sm hover:bg-muted/80",
            secondary:
                "bg-secondary text-secondary-foreground shadow-sm hover:bg-secondary/80",
            ghost: "hover:bg-secondary hover:text-secondary-foreground",
            accent: "bg-accent text-accent-foreground shadow-sm hover:bg-accent/80",
            link: "text-primary hover:text-primary/70 underline-offset-4 hover:underline shadow-none hover:shadow-none",
        },
        size: {
            default: "h-9 px-4 py-2",
            badge: "text-xs py-0.5 px-2",
            sm: "h-8 px-3 text-xs",
            lg: "h-10 px-8",
            icon: "h-9 w-9 min-w-9 min-h-9 [&_svg]:size-[1.375rem]",
        },
        shape: {
            default: "rounded-md",
            pill: "rounded-full",
        },
    },
    defaultVariants: {
        variant: "default",
        size: "default",
        shape: "pill",
    },
} as const;

const buttonVariants = cva(ButtonClasses.base, ButtonClasses);

export type ButtonProps = React.ButtonHTMLAttributes<HTMLButtonElement> &
    VariantProps<typeof buttonVariants> & {
        asChild?: boolean;
        loading?: boolean;
        startContent?: React.ReactNode;
        endContent?: React.ReactNode;
    };

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
    (
        {
            className,
            variant,
            size,
            shape,
            asChild = false,
            loading = false,
            startContent,
            endContent,
            children,
            ...props
        },
        ref
    ) => {
        const Comp = asChild ? Slot : "button";

        return (
            <Comp
                className={cn(
                    buttonVariants({ variant, size, shape, className })
                )}
                ref={ref}
                {...props}
            >
                {getContent()}
            </Comp>
        );

        function getContent() {
            if (size === "icon" && loading)
                return <Loader2 className="animate-spin" />;

            return loading ? (
                <div className="flex items-center gap-2">
                    <Loader2 className="mr-2 animate-spin" />
                    {children}
                    {endContent}
                </div>
            ) : (
                <div className="flex items-center gap-2">
                    {startContent}
                    {children}
                    {endContent}
                </div>
            );
        }
    }
);
Button.displayName = "Button";

export { Button, buttonVariants };
