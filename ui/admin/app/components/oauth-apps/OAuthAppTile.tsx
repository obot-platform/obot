import { OAuthProvider } from "~/lib/model/oauthApps/oauth-helpers";

import { Card } from "~/components/ui/card";
import { useOAuthAppInfo } from "~/hooks/oauthApps/useOAuthApps";

import { OAuthAppDetail } from "./OAuthAppDetail";

export function OAuthAppTile({ type }: { type: OAuthProvider }) {
    const info = useOAuthAppInfo(type);

    if (!info) {
        console.error(`OAuth app ${type} not found`);
        return null;
    }

    const { displayName } = info;

    return (
        <Card className="relative max-w-[300px] h-[150px] p-4 flex gap-4 justify-center items-center">
            <img
                src={info.logo}
                alt={displayName}
                className="dark:invert m-4"
            />

            <OAuthAppDetail type={type} className="absolute top-2 right-2" />
        </Card>
    );
}
