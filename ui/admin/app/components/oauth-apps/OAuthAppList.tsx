import { TypographyH3, TypographyP } from "~/components/Typography";
import { useOAuthAppList } from "~/hooks/oauthApps/useOAuthApps";

import { OAuthAppTile } from "./OAuthAppTile";

export function OAuthAppList() {
    const apps = useOAuthAppList();

    return (
        <div className="space-y-10 w-3/4 mx-auto">
            <div>
                <TypographyH3>Supported OAuth Apps</TypographyH3>

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
