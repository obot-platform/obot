import { OAuthProvider } from "~/lib/model/oauthApps/oauth-helpers";
import { cn } from "~/lib/utils";

import { TypographyH3 } from "~/components/Typography";
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
            className={cn("w-full flex flex-col", {
                "border-2 border-primary": info.appOverride,
            })}
        >
            <CardHeader className="flex flex-row justify-between items-start pb-2 space-y-0">
                <div className="flex flex-wrap gap-2 items-center">
                    <TypographyH3 className="min-w-fit">
                        {displayName}
                    </TypographyH3>

                    {info.appOverride ? (
                        <Tooltip>
                            <TooltipTrigger>
                                <Badge>Custom</Badge>
                            </TooltipTrigger>

                            <TooltipContent>
                                OAuth for {displayName} is configured by your
                                organization.
                            </TooltipContent>
                        </Tooltip>
                    ) : info.noGatewayIntegration ? (
                        <Tooltip>
                            <TooltipTrigger>
                                <Badge variant="secondary">
                                    Not Configured
                                </Badge>
                            </TooltipTrigger>

                            <TooltipContent>
                                OAuth for {displayName} is not configured
                            </TooltipContent>
                        </Tooltip>
                    ) : (
                        <Tooltip>
                            <TooltipTrigger>
                                <Badge variant="secondary">
                                    Default Configured
                                </Badge>
                            </TooltipTrigger>

                            <TooltipContent>
                                OAuth for {displayName} is handled by default by
                                the Obot Gateway
                            </TooltipContent>
                        </Tooltip>
                    )}
                </div>

                <OAuthAppDetail type={type} />
            </CardHeader>
            <CardContent className="flex-grow flex items-center justify-center">
                <div className="h-[100px] flex justify-center items-center overflow-clip">
                    <img
                        src={getSrc()}
                        alt={displayName}
                        className={cn("max-w-full max-h-[100px] aspect-auto", {
                            "dark:invert": info.invertDark,
                        })}
                    />
                </div>
            </CardContent>
        </Card>
    );
}
