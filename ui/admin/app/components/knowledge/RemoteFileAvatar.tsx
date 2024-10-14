import { Globe } from "lucide-react";

import { RemoteKnowledgeSourceType } from "~/lib/model/knowledge";

import { Avatar } from "~/components/ui/avatar";

export default function RemoteFileAvatar({
    remoteKnowledgeSourceType,
}: {
    remoteKnowledgeSourceType: RemoteKnowledgeSourceType;
}): React.ReactNode {
    const isOneDrive =
        remoteKnowledgeSourceType === RemoteKnowledgeSourceType.OneDrive;
    const isNotion =
        remoteKnowledgeSourceType === RemoteKnowledgeSourceType.Notion;
    const isWebsite =
        remoteKnowledgeSourceType === RemoteKnowledgeSourceType.Website;

    return (
        <>
            {isOneDrive && (
                <Avatar className="w-4 h-4 mr-2">
                    <img src="/onedrive.svg" alt="OneDrive logo" />
                </Avatar>
            )}
            {isNotion && (
                <Avatar className="w-4 h-4 mr-2">
                    <img src="/notion.svg" alt="Notion logo" />
                </Avatar>
            )}
            {isWebsite && (
                <Avatar className="w-4 h-4 mr-2">
                    <Globe className="w-4 h-4" />
                </Avatar>
            )}
        </>
    );
}
