import { useLoaderData } from "@remix-run/react";

import { OauthAppService } from "~/lib/service/api/oauthAppService";

import { CreateOauthApp } from "~/components/oauth-apps/CreateOauthApp";
import { OAuthAppList } from "~/components/oauth-apps/OAuthAppList";

export async function clientLoader() {
    const oauthApps = await OauthAppService.getOauthApps();
    const supportedApps = await OauthAppService.getSupportedOauthAppTypes();

    return { oauthApps, supportedApps };
}

export default function OauthApps() {
    const { supportedApps, oauthApps } = useLoaderData<typeof clientLoader>();

    console.log("oauthApps", supportedApps);

    return (
        <div className="h-full flex flex-col p-8 gap-8">
            <div className="flex justify-end">
                <CreateOauthApp spec={supportedApps} />
            </div>

            <OAuthAppList defaultData={oauthApps} spec={supportedApps} />
        </div>
    );
}
