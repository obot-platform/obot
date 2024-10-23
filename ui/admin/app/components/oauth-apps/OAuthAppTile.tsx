import { OAuthProvider } from "~/lib/model/oauthApps/oauth-helpers";
import { cn } from "~/lib/utils";

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

    if (!info) {
        console.error(`OAuth app ${type} not found`);
        return null;
    }

    const { displayName } = info;

    return (
        <Card
            className={cn(
                "self-center relative w-[300px] h-[150px] p-4 flex gap-4 justify-center items-center",
                className
            )}
        >
            <img
                src={info.logo}
                alt={displayName}
                className={cn("m-4", {
                    "dark:invert": info.invertDark,
                })}
            />

            {!info.disableConfiguration && (
                <OAuthAppDetail
                    type={type}
                    className="absolute top-2 right-2"
                />
            )}
        </Card>
    );
}
