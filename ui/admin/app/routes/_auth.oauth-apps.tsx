import { preload } from "swr";

import { OauthAppService } from "~/lib/service/api/oauthAppService";

import { TypographyH2 } from "~/components/Typography";
import { OAuthAppList } from "~/components/oauth-apps/OAuthAppList";
import { CreateCustomOAuthApp } from "~/components/oauth-apps/custom/CreateCustomOAuthApp";
import { CustomOAuthApps } from "~/components/oauth-apps/custom/CustomOAuthApps";

export async function clientLoader() {
    await preload(
        OauthAppService.getOauthApps.key(),
        OauthAppService.getOauthApps
    );

    return null;
}

export default function OauthApps() {
    return (
        <div className="relative space-y-10 px-8 pb-8">
            <div className="sticky top-0 bg-background pt-8 pb-4 flex items-center justify-between">
                <TypographyH2 className="mb-4">OAuth Apps</TypographyH2>

                <CreateCustomOAuthApp />
            </div>

            <div className="h-full flex flex-col gap-8 overflow-hidden">
                <OAuthAppList />

                <CustomOAuthApps />
            </div>
        </div>
    );
}
