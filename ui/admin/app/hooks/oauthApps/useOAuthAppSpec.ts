import useSWR from "swr";

import { OAuthAppSpec } from "~/lib/model/oauthApps";
import { OauthAppService } from "~/lib/service/api/oauthAppService";

type UseOAuthSpecReturn<T extends boolean> = T extends true
    ? OAuthAppSpec
    : OAuthAppSpec | undefined;

type UseOAuthSpecConfig<T extends boolean> = {
    isPreloaded: T;
};

export function useOAuthAppSpec<T extends boolean = false>(
    config?: UseOAuthSpecConfig<T>
): UseOAuthSpecReturn<T> {
    const { isPreloaded } = config ?? {};

    const { data: spec } = useSWR(
        OauthAppService.getSupportedOauthAppTypes.key(),
        OauthAppService.getSupportedOauthAppTypes
    );

    if (isPreloaded && !spec) {
        throw new Error("OAuth app spec is not preloaded");
    }

    return spec as UseOAuthSpecReturn<T>;
}
