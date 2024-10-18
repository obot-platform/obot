import { KeyIcon } from "lucide-react";
import { FaGithub } from "react-icons/fa";

import { OAuthProvider } from "~/lib/model/oauthApps/oauth-helpers";
import { cn } from "~/lib/utils";

const IconMap = {
    [OAuthProvider.GitHub]: FaGithub,
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
