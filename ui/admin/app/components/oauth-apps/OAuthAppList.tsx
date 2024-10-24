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

            <div className="flex flex-wrap gap-10">
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
