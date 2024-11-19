import { Slot } from "@radix-ui/react-slot";
import * as React from "react";

import { cn } from "~/lib/utils";

export interface ButtonDivProps extends React.HTMLAttributes<HTMLDivElement> {
    disabled?: boolean;
    "aria-label"?: string;
    asChild?: boolean;
}

const ClickableDiv = React.forwardRef<HTMLDivElement, ButtonDivProps>(
    (
        { className, disabled, onClick, children, asChild = false, ...props },
        ref
    ) => {
        const Comp = asChild ? Slot : "div";

        return (
            <Comp
                ref={ref}
                className={cn(
                    "select-none",
                    disabled && "pointer-events-none opacity-50",
                    className
                )}
                role="button"
                tabIndex={disabled ? -1 : 0}
                onClick={disabled ? undefined : onClick}
                aria-disabled={disabled}
                {...props}
            >
                {children}
            </Comp>
        );
    }
);

ClickableDiv.displayName = "ClickableDiv";

export { ClickableDiv };
