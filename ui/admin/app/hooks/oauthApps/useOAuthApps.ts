import { useMemo } from "react";
import useSWR from "swr";

import { combinedOAuthAppInfo } from "~/lib/model/oauthApps";
import { OAuthProvider } from "~/lib/model/oauthApps/oauth-helpers";
import { OauthAppService } from "~/lib/service/api/oauthAppService";

export function useOAuthAppList(config?: { revalidate?: boolean }) {
    const { revalidate = true } = config ?? {};

    const { data: apps } = useSWR(
        OauthAppService.getOauthApps.key(),
        OauthAppService.getOauthApps,
        { fallbackData: [], revalidateOnMount: revalidate }
    );

    const combinedApps = useMemo(() => combinedOAuthAppInfo(apps), [apps]);

    return combinedApps;
}

export function useCustomOAuthAppInfo() {
    const { data: apps } = useSWR(
        OauthAppService.getOauthApps.key(),
        OauthAppService.getOauthApps,
        { fallbackData: [] }
    );

    return apps.filter((app) => app.type === OAuthProvider.Custom);
}

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
