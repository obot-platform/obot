import { assetUrl, cn } from "~/lib/utils";

import { TypographyH2 } from "~/components/Typography";
import { useTheme } from "~/components/theme";

export function OttoLogo({
    hideText = false,
    classNames = {},
}: {
    hideText?: boolean;
    classNames?: { root?: string; image?: string };
}) {
    const { isDark } = useTheme();
    let logo = isDark
        ? "/logo/otto8-logo-blue-white-text.svg"
        : "/logo/otto8-logo-blue-black-text.svg";
    if (hideText) {
        logo = "/logo/otto8-icon-blue.svg";
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
                alt="Otto Logo"
                className={cn("h-8", classNames.image)}
            />
        </TypographyH2>
    );
}
