import { useLoaderData } from "@remix-run/react";
import { preload } from "swr";

import { OauthAppService } from "~/lib/service/api/oauthAppService";

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

    return null;
}

export default function OauthApps() {
    useLoaderData<typeof clientLoader>();

    return (
        <div className="h-full flex flex-col p-8 gap-8">
            <OAuthAppList />
        </div>
    );
}
