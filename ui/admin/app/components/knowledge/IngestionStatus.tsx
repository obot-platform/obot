import { CheckIcon } from "lucide-react";

import { KnowledgeFile, KnowledgeFileState } from "~/lib/model/knowledge";

import {
    Tooltip,
    TooltipContent,
    TooltipProvider,
    TooltipTrigger,
} from "~/components/ui/tooltip";

import { LoadingSpinner } from "../ui/LoadingSpinner";

interface IngestionStatusProps {
    files: KnowledgeFile[];
    ingestionError?: string;
}

const IngestionStatusComponent = ({
    files,
    ingestionError,
}: IngestionStatusProps) => {
    return (
        <div className="flex flex-col items-start mt-4">
            <div className="flex items-center">
                {(() => {
                    if (ingestionError) {
                        return (
                            <div className="flex items-center gap-2 max-w-[200px]">
                                <span className="text-xs text-destructive">
                                    {ingestionError}
                                </span>
                            </div>
                        );
                    }

                    const ingestingCount = files.filter(
                        (item) => item.state === KnowledgeFileState.Ingesting
                    ).length;
                    const queuedCount = files.filter(
                        (item) => item.state === KnowledgeFileState.Pending
                    ).length;
                    const ingestedCount = files.filter(
                        (item) => item.state === KnowledgeFileState.Ingested
                    ).length;
                    const totalCount = files.length;

                    if (ingestingCount > 0 || queuedCount > 0) {
                        return (
                            <>
                                <TooltipProvider>
                                    <Tooltip>
                                        <TooltipTrigger asChild>
                                            <div className="flex items-center">
                                                <LoadingSpinner className="w-4 h-4 mr-2" />
                                                <span className="text-sm text-gray-500">
                                                    Ingesting...
                                                </span>
                                            </div>
                                        </TooltipTrigger>
                                        <TooltipContent
                                            side="right"
                                            align="start"
                                            alignOffset={-8}
                                        >
                                            <p className="font-semibold">
                                                Ingestion Status:
                                            </p>
                                            <p>
                                                Files ingesting:{" "}
                                                {ingestingCount}
                                            </p>
                                            <p>
                                                Files ingested: {ingestedCount}
                                            </p>
                                            <p>Files queued: {queuedCount}</p>
                                        </TooltipContent>
                                    </Tooltip>
                                </TooltipProvider>
                            </>
                        );
                    } else if (
                        totalCount > 0 &&
                        queuedCount === 0 &&
                        ingestingCount === 0 &&
                        ingestedCount > 0
                    ) {
                        return (
                            <>
                                <CheckIcon className="w-4 h-4 text-green-500 mr-2" />
                                <span className="text-sm text-gray-500">
                                    {ingestedCount} file
                                    {ingestedCount !== 1 ? "s" : ""} ingested
                                </span>
                            </>
                        );
                    }
                    return null;
                })()}
            </div>
        </div>
    );
};

export default IngestionStatusComponent;
