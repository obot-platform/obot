import { Globe, UploadIcon } from "lucide-react";

import { RemoteKnowledgeSourceType } from "~/lib/model/knowledge";
import { assetUrl } from "~/lib/utils";

import { Avatar } from "~/components/ui/avatar";

export default function RemoteFileAvatar({
    knowledgeSourceType,
}: {
    knowledgeSourceType: RemoteKnowledgeSourceType | "files";
}): React.ReactNode {
    const isOneDrive =
        knowledgeSourceType === RemoteKnowledgeSourceType.OneDrive;
    const isNotion = knowledgeSourceType === RemoteKnowledgeSourceType.Notion;
    const isWebsite = knowledgeSourceType === RemoteKnowledgeSourceType.Website;
    const isUpload = knowledgeSourceType === "files";

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
