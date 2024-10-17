import useSWR from "swr";

import { OAuthAppInfo } from "~/lib/model/oauthApps";
import { OauthAppService } from "~/lib/service/api/oauthAppService";

const fallbackData = new Map<string, OAuthAppInfo>();

export function useOAuthAppSpec() {
    return useSWR(
        OauthAppService.getSupportedOauthAppTypes.key(),
        OauthAppService.getSupportedOauthAppTypes,
        { fallbackData }
    );
}
