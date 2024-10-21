import { useMemo } from "react";
import useSWR from "swr";

import { combinedOAuthAppInfo } from "~/lib/model/oauthApps";
import { OAuthProvider } from "~/lib/model/oauthApps/oauth-helpers";
import { OauthAppService } from "~/lib/service/api/oauthAppService";

const key = () => ({
    ...OauthAppService.getOauthApps.key(),
    modifier: "combinedList",
});

export function useOAuthAppList(config?: { revalidate?: boolean }) {
    const { revalidate = true } = config ?? {};

    const { data: apps } = useSWR(
        key(),
        async () => combinedOAuthAppInfo(await OauthAppService.getOauthApps()),
        { fallbackData: [], revalidateOnMount: revalidate }
    );

    return apps;
}
useOAuthAppList.key = key;

export function useOAuthAppInfo(type: OAuthProvider) {
    const list = useOAuthAppList({ revalidate: false });

    const app = useMemo(
        () => list.find((app) => app.type === type),
        [list, type]
    );

    if (!app) {
        throw new Error(`OAuth app ${type} not found`);
    }

    return app;
}
