import { cn } from "~/lib/utils";

import { TypographyH2 } from "~/components/Typography";

export function OttoLogo({
    hideText = false,
    classNames = {},
}: {
    hideText?: boolean;
    classNames?: { root?: string; image?: string };
}) {
    return (
        <TypographyH2
            className={cn(
                "text-center flex gap-2 items-center justify-center pb-0",
                classNames.root
            )}
        >
            <img
                src="/logo/OttoLogo.svg"
                alt="Otto Logo"
                className={cn("w-10 h-10 dark:invert", classNames.image)}
            />
            {!hideText && "Otto"}
        </TypographyH2>
    );
}
