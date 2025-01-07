import { assetUrl, cn } from "~/lib/utils";

import { useTheme } from "~/components/theme";

export function ObotLogo({
    hideText = false,
    classNames = {},
}: {
    hideText?: boolean;
    classNames?: { root?: string; image?: string };
}) {
    const { isDark } = useTheme();
    let logo = isDark
        ? "/logo/obot-logo-blue-white-text.svg"
        : "/logo/obot-logo-blue-black-text.svg";
    if (hideText) {
        logo = "/logo/obot-icon-blue.svg";
    }
    return (
        <h2
            className={cn(
                "text-center flex gap-2 items-center justify-center pb-0",
                classNames.root
            )}
        >
            <img
                src={assetUrl(logo)}
                alt="Obot Logo"
                className={cn("h-8", classNames.image)}
            />
        </h2>
    );
}
