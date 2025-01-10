import { OAuthAppTile } from "~/components/oauth-apps/OAuthAppTile";
import { useOAuthAppList } from "~/hooks/oauthApps/useOAuthApps";

export function OAuthAppList() {
	const apps = useOAuthAppList();

	return (
		<div className="space-y-4">
			<h3>Default OAuth Apps</h3>

			<div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4">
				{apps.map(({ type }) => (
					<OAuthAppTile
						key={type}
						type={type}
						className="justify-self-center"
					/>
				))}
			</div>
		</div>
	);
}
