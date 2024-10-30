import { TypographyH3 } from "~/components/Typography";
import { useCustomOAuthAppInfo } from "~/hooks/oauthApps/useOAuthApps";

import { CustomOAuthAppTile } from "./CustomOAuthAppTile";

export function CustomOAuthApps() {
    const apps = useCustomOAuthAppInfo();

    return (
        <div className="space-y-4">
            <TypographyH3>Custom OAuth Apps</TypographyH3>

            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4 gap-4">
                {apps.map((app) => (
                    <CustomOAuthAppTile app={app} key={app.refName} />
                ))}
            </div>
        </div>
    );
}
