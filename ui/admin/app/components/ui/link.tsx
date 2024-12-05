import {
    Link as RemixLink,
    LinkProps as RemixLinkProps,
} from "@remix-run/react";
import { VariantProps, cva } from "class-variance-authority";
import { forwardRef } from "react";

import { cn } from "~/lib/utils";

import { buttonVariants } from "~/components/ui/button";

const linkVariants = cva("", {
    variants: {
        as: {
            default: buttonVariants({
                variant: "link",
                size: "none",
                shape: "none",
            }),
            button: "flex flex-row items-center gap-2",
            div: "",
        },
    },
    defaultVariants: {
        as: "default",
    },
});

type LinkVariants = VariantProps<typeof linkVariants>;
type ButtonVariants = VariantProps<typeof buttonVariants>;

export type LinkProps = RemixLinkProps & LinkVariants & ButtonVariants;

export const Link = forwardRef<HTMLAnchorElement, LinkProps>(
    ({ as, variant, size, shape, className, ...rest }, ref) => (
        <RemixLink
            {...rest}
            ref={ref}
            className={cn(
                linkVariants({ as }),
                as === "button" && buttonVariants({ variant, size, shape }),
                className
            )}
        />
    )
);

Link.displayName = "Link";
