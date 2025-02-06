import { OAuthProvider } from "~/lib/model/oauthApps/oauth-helpers";
import { ToolReference } from "~/lib/model/toolReferences";

import { CustomOauthAppDetail } from "~/components/oauth-apps/shared/CustomOauthAppDetail";
import { OAuthAppDetail } from "~/components/oauth-apps/shared/OAuthAppDetail";
import { useOauthAppMap } from "~/hooks/oauthApps/useOAuthApps";

type ToolOauthConfigProps = {
	tool?: ToolReference;
	open: boolean;
	onOpenChange: (open: boolean) => void;
	onSuccess?: () => void;
};

export function ToolOauthConfig({
	tool,
	open,
	onOpenChange,
	onSuccess,
}: ToolOauthConfigProps) {
	const oauthAppsMap = useOauthAppMap();
	const oauthMetadata = tool?.metadata?.oauth;
	if (!oauthMetadata) return null;

	const oauth = oauthAppsMap.get(tool?.metadata?.oauth ?? "");
	const isSpecedOauth =
		oauthMetadata &&
		Object.values(OAuthProvider).includes(oauthMetadata as OAuthProvider);

	return isSpecedOauth ? (
		<OAuthAppDetail
			open={open}
			onOpenChange={onOpenChange}
			onSuccess={onSuccess}
			type={oauthMetadata as OAuthProvider}
		/>
	) : (
		<CustomOauthAppDetail
			open={open}
			onOpenChange={onOpenChange}
			app={oauth}
			alias={oauthMetadata}
		/>
	);
}
