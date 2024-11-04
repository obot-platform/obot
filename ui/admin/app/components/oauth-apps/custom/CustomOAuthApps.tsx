import { TypographyH3 } from "~/components/Typography";
import { CustomOAuthAppTile } from "~/components/oauth-apps/custom/CustomOAuthAppTile";
import { useCustomOAuthAppInfo } from "~/hooks/oauthApps/useOAuthApps";

export function CustomOAuthApps() {
    const apps = useCustomOAuthAppInfo();

    if (apps.length === 0) return null;

    return (
        <div className="space-y-4">
            <TypographyH3>Custom OAuth Apps</TypographyH3>

            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4 gap-4">
                {apps.map((app) => (
                    <CustomOAuthAppTile app={app} key={app.id} />
                ))}
            </div>
        </div>
    );
}
