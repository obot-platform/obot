import { OAuthProvider } from "~/lib/model/oauthApps/oauth-helpers";
import { cn } from "~/lib/utils";

import { OAuthAppDetail } from "~/components/oauth-apps/OAuthAppDetail";
import { useTheme } from "~/components/theme";
import { Badge } from "~/components/ui/badge";
import { Card, CardContent, CardHeader } from "~/components/ui/card";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "~/components/ui/tooltip";
import { useOAuthAppInfo } from "~/hooks/oauthApps/useOAuthApps";

export function OAuthAppTile({
	type,
}: {
	type: OAuthProvider;
	className?: string;
}) {
	const info = useOAuthAppInfo(type);
	const { isDark } = useTheme();

	if (!info) {
		console.error(`OAuth app ${type} not found`);
		return null;
	}

	const { displayName } = info;

	const getSrc = () => {
		if (isDark) return info.darkLogo ?? info.logo;
		return info.logo;
	};

	return (
		<Card
			className={cn("flex w-full flex-col", {
				"border-2 border-primary": info.appOverride,
			})}
		>
			<CardHeader className="flex flex-row items-start justify-between space-y-0 pb-2">
				<div className="flex flex-wrap items-center gap-2">
					<h3 className="min-w-fit">{displayName}</h3>

					{info.appOverride ? (
						<Tooltip>
							<TooltipTrigger>
								<Badge>Custom</Badge>
							</TooltipTrigger>

							<TooltipContent>
								OAuth for {displayName} is configured by your organization.
							</TooltipContent>
						</Tooltip>
					) : info.noGatewayIntegration ? (
						<Tooltip>
							<TooltipTrigger>
								<Badge variant="secondary">Not Configured</Badge>
							</TooltipTrigger>

							<TooltipContent>
								OAuth for {displayName} is not configured
							</TooltipContent>
						</Tooltip>
					) : (
						<Tooltip>
							<TooltipTrigger>
								<Badge variant="secondary">Default Configured</Badge>
							</TooltipTrigger>

							<TooltipContent>
								OAuth for {displayName} is handled by default by the Obot
								Gateway
							</TooltipContent>
						</Tooltip>
					)}
				</div>

				<OAuthAppDetail type={type} />
			</CardHeader>
			<CardContent className="flex flex-grow items-center justify-center">
				<div className="flex h-[100px] items-center justify-center overflow-clip">
					<img
						src={getSrc()}
						alt={displayName}
						className={cn("aspect-auto max-h-[100px] max-w-full", {
							"dark:invert": info.invertDark,
						})}
					/>
				</div>
			</CardContent>
		</Card>
	);
}
