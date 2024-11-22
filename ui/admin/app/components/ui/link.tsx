import {
    Link as RemixLink,
    LinkProps as RemixLinkProps,
} from "@remix-run/react";
import { VariantProps, cva } from "class-variance-authority";

import { cn } from "~/lib/utils";

import { ButtonClasses } from "~/components/ui/button";

const linkVariants = cva("", {
    variants: {
        as: {
            button: cn(ButtonClasses.base, "flex flex-row items-center gap-2"),
            default: ButtonClasses.variants.variant.link,
            div: "",
        },
        buttonVariant: ButtonClasses.variants.variant,
        buttonSize: ButtonClasses.variants.size,
        buttonShape: ButtonClasses.variants.shape,
    },
    defaultVariants: {
        as: "default",
    },
});

type LinkVariants = VariantProps<typeof linkVariants>;

export type LinkProps = RemixLinkProps & LinkVariants;

export function Link({
    as,
    buttonVariant,
    buttonSize,
    buttonShape,
    className,
    ...rest
}: LinkProps) {
    const buttonVariants = getButtonVariants({
        as,
        buttonVariant,
        buttonSize,
        buttonShape,
    });

    return (
        <RemixLink
            {...rest}
            className={linkVariants({ as, ...buttonVariants, className })}
        />
    );

    function getButtonVariants(props: LinkVariants) {
        if (props.as !== "button") return {};

        return {
            buttonVariant:
                props.buttonVariant ?? ButtonClasses.defaultVariants.variant,
            buttonSize: props.buttonSize ?? ButtonClasses.defaultVariants.size,
            buttonShape:
                props.buttonShape ?? ButtonClasses.defaultVariants.shape,
        };
    }
}
