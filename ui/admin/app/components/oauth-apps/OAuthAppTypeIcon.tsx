import { NotionLogoIcon } from "@radix-ui/react-icons";
import { KeyIcon } from "lucide-react";
import { FaGithub, FaGoogle, FaMicrosoft, FaSlack } from "react-icons/fa";

import { OAuthProvider } from "~/lib/model/oauthApps/oauth-helpers";
import { cn } from "~/lib/utils";

const IconMap = {
    [OAuthProvider.GitHub]: FaGithub,
    [OAuthProvider.Slack]: FaSlack,
    [OAuthProvider.Google]: FaGoogle,
    [OAuthProvider.Microsoft365]: FaMicrosoft,
    [OAuthProvider.Notion]: NotionLogoIcon,
    [OAuthProvider.Custom]: KeyIcon,
};

export function OAuthAppTypeIcon({
    type,
    className,
}: {
    type: OAuthProvider;
    className?: string;
}) {
    const Icon = IconMap[type] ?? KeyIcon;

    return <Icon className={cn("w-6 h-6", className)} />;
}
