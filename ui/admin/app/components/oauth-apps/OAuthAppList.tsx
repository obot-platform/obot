import { TypographyH3 } from "~/components/Typography";
import { useOAuthAppList } from "~/hooks/oauthApps/useOAuthApps";

import { OAuthAppTile } from "./OAuthAppTile";

export function OAuthAppList() {
    const apps = useOAuthAppList();

    return (
        <div className="space-y-4">
            <TypographyH3>Default OAuth Apps</TypographyH3>

            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4 gap-4">
                {apps.map(({ type }) => (
                    <OAuthAppTile
                        key={type}
                        type={type}
                        className="justify-self-center"
                    />
                ))}
            </div>
        </div>
    );
}
