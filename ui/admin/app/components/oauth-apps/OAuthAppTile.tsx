import { OAuthProvider } from "~/lib/model/oauthApps/oauth-helpers";
import { cn } from "~/lib/utils";

import { useTheme } from "~/components/theme";
import { Card } from "~/components/ui/card";
import { useOAuthAppInfo } from "~/hooks/oauthApps/useOAuthApps";

import { OAuthAppDetail } from "./OAuthAppDetail";

export function OAuthAppTile({
    type,
    className,
}: {
    type: OAuthProvider;
    className?: string;
}) {
    const info = useOAuthAppInfo(type);
    const { isDark } = useTheme();

    if (!info) {
        console.error(`OAuth app ${type} not found`);
        return null;
    }

    const { displayName } = info;

    if (info.type == "slack") {
        console.log(info);
    }

    const getSrc = () => {
        if (isDark) return info.darkLogo ?? info.logo;
        return info.logo;
    };

    return (
        <Card
            className={cn(
                "self-center relative w-[300px] h-[150px] px-6 flex gap-4 justify-center items-center",
                className
            )}
        >
            <img
                src={getSrc()}
                alt={displayName}
                className={cn("m-4 aspect-auto", {
                    "dark:invert": info.invertDark,
                })}
            />

            <OAuthAppDetail type={type} className="absolute top-2 right-2" />
        </Card>
    );
}
