import { assetUrl, cn } from "~/lib/utils";

import { TypographyH2 } from "~/components/Typography";
import { useTheme } from "~/components/theme";

export function AcornLogo({
    hideText = false,
    classNames = {},
}: {
    hideText?: boolean;
    classNames?: { root?: string; image?: string };
}) {
    const { isDark } = useTheme();
    let logo = isDark
        ? "/logo/acorn-logo-blue-white-text.svg"
        : "/logo/acorn-logo-blue-black-text.svg";
    if (hideText) {
        logo = "/logo/acorn-icon-blue.svg";
    }
    return (
        <TypographyH2
            className={cn(
                "text-center flex gap-2 items-center justify-center pb-0",
                classNames.root
            )}
        >
            <img
                src={assetUrl(logo)}
                alt="Acorn Logo"
                className={cn("h-8", classNames.image)}
            />
        </TypographyH2>
    );
}
