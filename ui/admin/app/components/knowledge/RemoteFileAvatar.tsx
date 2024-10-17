import { Globe, UploadIcon } from "lucide-react";

import { RemoteKnowledgeSourceType } from "~/lib/model/knowledge";
import { assetUrl } from "~/lib/utils";

import { Avatar } from "~/components/ui/avatar";

export default function RemoteFileAvatar({
    remoteKnowledgeSourceType,
}: {
    remoteKnowledgeSourceType: RemoteKnowledgeSourceType | "files";
}): React.ReactNode {
    const isOneDrive =
        remoteKnowledgeSourceType === RemoteKnowledgeSourceType.OneDrive;
    const isNotion =
        remoteKnowledgeSourceType === RemoteKnowledgeSourceType.Notion;
    const isWebsite =
        remoteKnowledgeSourceType === RemoteKnowledgeSourceType.Website;
    const isUpload = remoteKnowledgeSourceType === "files";

    return (
        <>
            {isOneDrive && (
                <Avatar className="w-4 h-4 mr-2">
                    <img src={assetUrl("/onedrive.svg")} alt="OneDrive logo" />
                </Avatar>
            )}
            {isNotion && (
                <Avatar className="w-4 h-4 mr-2">
                    <img src={assetUrl("/notion.svg")} alt="Notion logo" />
                </Avatar>
            )}
            {isWebsite && (
                <Avatar className="w-4 h-4 mr-2">
                    <Globe className="w-4 h-4" />
                </Avatar>
            )}
            {isUpload && (
                <Avatar className="w-4 h-4 mr-2">
                    <UploadIcon className="w-4 h-4" />
                </Avatar>
            )}
        </>
    );
}
