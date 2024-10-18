import { useOAuthAppList } from "~/hooks/oauthApps";

import { TypographyH3, TypographyP } from "../Typography";
import { OAuthAppTile } from "./OAuthAppTile";

export function OAuthAppList() {
    const apps = useOAuthAppList();

    return (
        <div className="space-y-10 w-3/4 mx-auto">
            <div>
                <TypographyH3>Pre-configured OAuth Apps</TypographyH3>
                <TypographyP className="!mt-0">
                    These apps are pre-configured and ready to use. For the most
                    part, you should not need to configure any additional OAuth
                    apps.
                </TypographyP>
            </div>

            <div className="grid grid-cols-2 gap-10 lg:grid-cols-3 xl:grid-cols-4">
                {apps.map(({ type }) => (
                    <OAuthAppTile key={type} type={type} />
                ))}
            </div>
        </div>
    );
}
