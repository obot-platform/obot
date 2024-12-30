import { type VariantProps, cva } from "class-variance-authority";
import { EyeIcon, EyeOffIcon } from "lucide-react";
import * as React from "react";

import { cn } from "~/lib/utils";

import { buttonVariants } from "~/components/ui/button";

const InputReset =
    "w-full p-3 bg-transparent border-none focus-visible:border-none focus-visible:outline-none rounded-full";

const inputVariants = cva(
    cn(
        "flex items-center h-9 w-full rounded-md bg-transparent border border-input text-sm transition-colors file:border-0 file:bg-transparent file:text-sm file:font-medium placeholder:text-muted-foreground has-[:focus-visible]:ring-1 has-[:focus-visible]:ring-ring has-[:disabled]:cursor-not-allowed has-[:disabled]:opacity-50"
    ),
    {
        variants: {
            variant: {
                default: "",
                ghost: "shadow-none cursor-pointer hover:border-primary px-0 mb-0 font-bold outline-none border-transparent has-[:focus-visible]:border-primary",
            },
        },
        defaultVariants: {
            variant: "default",
        },
    }
);

export interface InputProps
    extends React.InputHTMLAttributes<HTMLInputElement>,
        VariantProps<typeof inputVariants> {
    disableToggle?: boolean;
}

const Input = React.forwardRef<HTMLInputElement, InputProps>(
    ({ className, variant, type, disableToggle = false, ...props }, ref) => {
        const isPassword = type === "password";
        const [isVisible, setIsVisible] = React.useState(false);

        const internalType = isPassword
            ? isVisible && !disableToggle
                ? "text"
                : "password"
            : type;

        const toggleVisible = React.useCallback(
            () => setIsVisible((prev) => !prev),
            []
        );

        return (
            <div className={cn(inputVariants({ variant, className }))}>
                <input
                    type={internalType}
                    data-1p-ignore={!isPassword}
                    className={InputReset}
                    ref={ref}
                    {...props}
                />

                {isPassword && !disableToggle && (
                    <button
                        type="button"
                        onClick={toggleVisible}
                        className={buttonVariants({
                            variant: "ghost",
                            size: "icon",
                            shape: "default",
                            className: "min-h-full !h-full rounded-s-none",
                        })}
                    >
                        {!isVisible ? <EyeIcon /> : <EyeOffIcon />}
                    </button>
                )}
            </div>
        );
    }
);
Input.displayName = "Input";

export { Input, inputVariants };
