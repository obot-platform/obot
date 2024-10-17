import { CheckIcon } from "lucide-react";

import { IngestionStatus, KnowledgeFile } from "~/lib/model/knowledge";

import {
    Tooltip,
    TooltipContent,
    TooltipProvider,
    TooltipTrigger,
} from "~/components/ui/tooltip";

import { LoadingSpinner } from "../ui/LoadingSpinner";
import RemoteKnowledgeSourceStatus from "./RemoteKnowledgeSourceStatus";

interface IngestionStatusProps {
    knowledge: KnowledgeFile[];
}

const IngestionStatusComponent = ({ knowledge }: IngestionStatusProps) => {
    const ingestingCount = knowledge.filter(
        (item) =>
            item.ingestionStatus?.status === IngestionStatus.Starting ||
            item.ingestionStatus?.status === IngestionStatus.Completed
    ).length;
    const queuedCount = knowledge.filter(
        (item) => item.ingestionStatus?.status === IngestionStatus.Queued
    ).length;
    const notSupportedCount = knowledge.filter(
        (item) => item.ingestionStatus?.status === IngestionStatus.Unsupported
    ).length;
    const ingestedCount = knowledge.filter(
        (item) =>
            item.ingestionStatus?.status === IngestionStatus.Finished ||
            item.ingestionStatus?.status === IngestionStatus.Skipped
    ).length;
    const totalCount = knowledge.length;

    return (
        <div className="flex flex-col items-start mt-4">
            <div className="flex items-center">
                {(() => {
                    const ingestingCount = knowledge.filter(
                        (item) =>
                            item.ingestionStatus?.status ===
                                IngestionStatus.Starting ||
                            item.ingestionStatus?.status ===
                                IngestionStatus.Completed
                    ).length;
                    const queuedCount = knowledge.filter(
                        (item) =>
                            item.ingestionStatus?.status ===
                            IngestionStatus.Queued
                    ).length;
                    const notSupportedCount = knowledge.filter(
                        (item) =>
                            item.ingestionStatus?.status ===
                            IngestionStatus.Unsupported
                    ).length;
                    const ingestedCount = knowledge.filter(
                        (item) =>
                            item.ingestionStatus?.status ===
                                IngestionStatus.Finished ||
                            item.ingestionStatus?.status ===
                                IngestionStatus.Skipped
                    ).length;
                    const totalCount = knowledge.length;

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
                                            <p>
                                                Files not supported:{" "}
                                                {notSupportedCount}
                                            </p>
                                        </TooltipContent>
                                    </Tooltip>
                                </TooltipProvider>
                            </>
                        );
                    } else if (
                        totalCount > 0 &&
                        queuedCount === 0 &&
                        ingestingCount === 0
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
