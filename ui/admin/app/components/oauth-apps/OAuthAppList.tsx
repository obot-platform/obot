import { TypographyH2, TypographyP } from "~/components/Typography";
import { useOAuthAppList } from "~/hooks/oauthApps/useOAuthApps";

import { OAuthAppTile } from "./OAuthAppTile";

export function OAuthAppList() {
    const apps = useOAuthAppList();

    return (
        <div className="space-y-10">
            <div>
                <TypographyH2 className="mb-4">
                    Supported OAuth Apps
                </TypographyH2>

                <TypographyP className="!mt-0">
                    These are the currently supported OAuth apps for Otto. These
                    are here to allow users to access the following services via
                    tools.
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
