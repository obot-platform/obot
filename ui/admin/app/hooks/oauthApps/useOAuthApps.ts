import { useMemo } from "react";
import useSWR from "swr";

import { OAuthProvider } from "~/lib/model/oauthApps/oauth-helpers";
import { OauthAppService } from "~/lib/service/api/oauthAppService";

export function useOAuthAppList(config?: { revalidate?: boolean }) {
	const { revalidate = true } = config ?? {};

	const { data: apps } = useSWR(
		OauthAppService.getOauthApps.key(),
		OauthAppService.getOauthApps,
		{
			fallbackData: [],
			revalidateOnMount: revalidate,
		}
	);

	return apps;
}

export function useOAuthAppInfo(type: OAuthProvider) {
	const list = useOAuthAppList({ revalidate: false });

	const app = useMemo(
		() => list.find((app) => app.type === type),
		[list, type]
	);

	return app;
}
