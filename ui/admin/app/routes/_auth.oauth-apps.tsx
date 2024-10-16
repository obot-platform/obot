import { preload } from "swr";

import { OauthAppService } from "~/lib/service/api/oauthAppService";

import { CreateOauthApp } from "~/components/oauth-apps/CreateOauthApp";
import { OAuthAppList } from "~/components/oauth-apps/OAuthAppList";

export async function clientLoader() {
    await Promise.all([
        preload(
            OauthAppService.getSupportedOauthAppTypes.key(),
            OauthAppService.getSupportedOauthAppTypes
        ),
        preload(
            OauthAppService.getOauthApps.key(),
            OauthAppService.getOauthApps
        ),
    ]);
}

export default function OauthApps() {
    return (
        <div className="h-full flex flex-col p-8 gap-8">
            <div className="flex justify-end">
                <CreateOauthApp />
            </div>

            <OAuthAppList />
        </div>
    );
}
