import React from "react";

import {
    KnowledgeSource,
    RemoteKnowledgeSourceType,
} from "~/lib/model/knowledge";

import { LoadingSpinner } from "~/components/ui/LoadingSpinner";

import RemoteFileAvatar from "./RemoteFileAvatar";

interface RemoteKnowledgeSourceStatusProps {
    source: KnowledgeSource | undefined;
    sourceType: RemoteKnowledgeSourceType;
}

const RemoteKnowledgeSourceStatus: React.FC<
    RemoteKnowledgeSourceStatusProps
> = ({ source, sourceType }) => {
    return (
        <div className="flex flex-row mt-2 flex items-center">
            {source?.state === "syncing" && (
                <>
                    <RemoteFileAvatar knowledgeSourceType={sourceType} />
                    <span className="text-sm mr-2 text-gray-500">
                        {source.status || "Syncing Files..."}
                    </span>
                    <LoadingSpinner className="w-4 h-4" />
                </>
            )}
            {source?.state === "error" && (
                <span className="text-sm mr-2 text-destructive">
                    {source.error}
                </span>
            )}
        </div>
    );
};

export default RemoteKnowledgeSourceStatus;
