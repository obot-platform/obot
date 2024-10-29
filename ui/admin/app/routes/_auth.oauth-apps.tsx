import { preload } from "swr";

import { OauthAppService } from "~/lib/service/api/oauthAppService";

import { OAuthAppList } from "~/components/oauth-apps/OAuthAppList";
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
        <div className="h-full flex flex-col p-8 gap-8">
            <OAuthAppList />

            <CustomOAuthApps />
        </div>
    );
}
