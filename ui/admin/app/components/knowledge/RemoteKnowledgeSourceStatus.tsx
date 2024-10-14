import React from "react";

import { RemoteKnowledgeSource } from "~/lib/model/knowledge";

import { LoadingSpinner } from "~/components/ui/LoadingSpinner";

import RemoteFileAvatar from "./RemoteFileAvatar";

interface RemoteKnowledgeSourceStatusProps {
    source: RemoteKnowledgeSource;
}

const RemoteKnowledgeSourceStatus: React.FC<
    RemoteKnowledgeSourceStatusProps
> = ({ source }) => {
    if (!source) return null;
    return (
        <div
            key={source.id}
            className="flex flex-row justify-center items-center mt-2"
        >
            <div className="flex items-center">
                <RemoteFileAvatar
                    remoteKnowledgeSourceType={source.sourceType!}
                />
                <span className="text-sm text-gray-500 mr-2">
                    {source?.status || "Syncing Files..."}
                </span>
                <LoadingSpinner className="w-4 h-4" />
            </div>
        </div>
    );
};

export default RemoteKnowledgeSourceStatus;
