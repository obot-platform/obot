import React from "react";

import { RemoteKnowledgeSource } from "~/lib/model/knowledge";

import { LoadingSpinner } from "~/components/ui/LoadingSpinner";

import RemoteFileAvatar from "./RemoteFileAvatar";

interface RemoteKnowledgeSourceStatusProps {
    source: RemoteKnowledgeSource | undefined;
    includeAvatar?: boolean;
}

const RemoteKnowledgeSourceStatus: React.FC<
    RemoteKnowledgeSourceStatusProps
> = ({ source, includeAvatar = true }) => {
    if (!source || (!source.runID && !source.error)) return null;

    if (source.sourceType === "onedrive" && !source.onedriveConfig) return null;

    if (source.sourceType === "website" && !source.websiteCrawlingConfig)
        return null;

    return (
        <div key={source.id} className="flex flex-row mt-2">
            <div className="flex items-center">
                {includeAvatar && (
                    <RemoteFileAvatar
                        remoteKnowledgeSourceType={source.sourceType!}
                    />
                )}
                <span
                    className={`text-sm mr-2 ${source?.error ? "text-destructive" : "text-gray-500"}`}
                >
                    {source?.error || source?.status || "Syncing Files..."}
                </span>
                {!source.error && <LoadingSpinner className="w-4 h-4" />}
            </div>
        </div>
    );
};

export default RemoteKnowledgeSourceStatus;
