import { useCustomOAuthAppInfo } from "~/hooks/oauthApps/useOAuthApps";

export function CustomOAuthApps() {
    const apps = useCustomOAuthAppInfo();

    return <div>{apps.length}</div>;
}
