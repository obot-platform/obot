import { MetaFunction } from "react-router";
import { preload } from "swr";

import { OauthAppService } from "~/lib/service/api/oauthAppService";
import { RouteHandle } from "~/lib/service/routeHandles";

import { OAuthAppList } from "~/components/oauth-apps/OAuthAppList";
import { CreateCustomOAuthApp } from "~/components/oauth-apps/custom/CreateCustomOAuthApp";
import { CustomOAuthApps } from "~/components/oauth-apps/custom/CustomOAuthApps";

export async function clientLoader() {
	await preload(
		OauthAppService.getOauthApps.key(),
		OauthAppService.getOauthApps
	);

	return null;
}

export default function OauthApps() {
	return (
		<div className="relative space-y-10 px-8 pb-8">
			<div className="sticky top-0 flex items-center justify-between bg-background pb-4 pt-8">
				<h2 className="mb-4">OAuth Apps</h2>

				<CreateCustomOAuthApp />
			</div>

			<div className="flex h-full flex-col gap-8 overflow-hidden">
				<OAuthAppList />

				<CustomOAuthApps />
			</div>
		</div>
	);
}

export const handle: RouteHandle = {
	breadcrumb: () => [{ content: "OAuth Apps" }],
};

export const meta: MetaFunction = () => {
	return [{ title: `Obot • Oauth Apps` }];
};
