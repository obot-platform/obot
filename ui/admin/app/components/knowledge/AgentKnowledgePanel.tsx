import { CheckIcon, Info, PlusIcon, XCircleIcon } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import useSWR from "swr";

import {
    IngestionStatus,
    KnowledgeFile,
    KnowledgeIngestionStatus,
    getIngestionStatus,
    getMessage,
    getRemoteFileDisplayName,
} from "~/lib/model/knowledge";
import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { KnowledgeService } from "~/lib/service/api/knowledgeService";
import { cn, getErrorMessage } from "~/lib/utils";

import { Button } from "~/components/ui/button";
import { ScrollArea } from "~/components/ui/scroll-area";
import {
    Tooltip,
    TooltipContent,
    TooltipProvider,
    TooltipTrigger,
} from "~/components/ui/tooltip";
import { useAsync } from "~/hooks/useAsync";
import { useMultiAsync } from "~/hooks/useMultiAsync";

import { LoadingSpinner } from "../ui/LoadingSpinner";
import { Input } from "../ui/input";
import { AddFileModal } from "./AddFileModal";
import { FileChip } from "./FileItem";
import RemoteFileItemChip from "./RemoteFileItemChip";
import RemoteKnowledgeSourceStatus from "./RemoteKnowledgeSourceStatus";

export function AgentKnowledgePanel({
    agentId,
    className,
}: {
    agentId: string;
    className?: string;
}) {
    const [blockPolling, setBlockPolling] = useState(false);
    const [isAddFileModalOpen, setIsAddFileModalOpen] = useState(false);

    const getKnowledge = useSWR(
        KnowledgeService.getKnowledgeForAgent.key(agentId),
        ({ agentId }) =>
            KnowledgeService.getKnowledgeForAgent(agentId).then((items) =>
                items
                    .sort((a, b) => a.fileName.localeCompare(b.fileName))
                    .map(
                        (item) =>
                            ({
                                ...item,
                                ingestionStatus: {
                                    ...item.ingestionStatus,
                                    status: getIngestionStatus(
                                        item.ingestionStatus
                                    ),
                                },
                            }) as KnowledgeFile
                    )
            ),
        {
            revalidateOnFocus: false,
            // poll every second for ingestion status updates unless blocked
            refreshInterval: blockPolling ? undefined : 1000,
        }
    );
    const knowledge = getKnowledge.data || [];

    const getRemoteKnowledgeSources = useSWR(
        KnowledgeService.getRemoteKnowledgeSource.key(agentId),
        ({ agentId }) => KnowledgeService.getRemoteKnowledgeSource(agentId),
        {
            revalidateOnFocus: false,
            refreshInterval: 5000,
        }
    );
    const remoteKnowledgeSources = useMemo(
        () => getRemoteKnowledgeSources.data || [],
        [getRemoteKnowledgeSources.data]
    );

    const deleteKnowledge = useAsync(async (item: KnowledgeFile) => {
        await KnowledgeService.deleteKnowledgeFromAgent(agentId, item.fileName);

        const remoteKnowledgeSource = remoteKnowledgeSources?.find(
            (source) => source.sourceType === item.remoteKnowledgeSourceType
        );
        if (remoteKnowledgeSource) {
            await KnowledgeService.updateRemoteKnowledgeSource(
                agentId,
                remoteKnowledgeSource.id,
                {
                    ...remoteKnowledgeSource,
                    exclude: [
                        ...(remoteKnowledgeSource.exclude || []),
                        item.uploadID || "",
                    ],
                }
            );
        }

        // optomistic update without cache revalidation
        getKnowledge.mutate((prev) =>
            prev?.filter((prevItem) => prevItem.fileName !== item.fileName)
        );
    });

    const handleAddKnowledge = useCallback(
        async (_index: number, file: File) => {
            await new Promise((resolve) => setTimeout(resolve, 1000));
            await KnowledgeService.addKnowledgeToAgent(agentId, file);

            // once added, we can immediately mutate the cache value
            // without revalidating.
            // Revalidating here would cause knowledge to be refreshed
            // for each file being uploaded, which is not desirable.
            const newItem: KnowledgeFile = {
                fileName: file.name,
                agentID: agentId,
                // set ingestion status to starting to ensure polling is enabled
                ingestionStatus: { status: IngestionStatus.Queued },
                fileDetails: {},
            };

            getKnowledge.mutate(
                (prev) => {
                    const existingItemIndex = prev?.findIndex(
                        (item) => item.fileName === newItem.fileName
                    );
                    if (existingItemIndex !== -1 && prev) {
                        const updatedPrev = [...prev];
                        updatedPrev[existingItemIndex!] = newItem;
                        return updatedPrev;
                    } else {
                        return [newItem, ...(prev || [])];
                    }
                },
                {
                    revalidate: false,
                }
            );
        },
        [agentId, getKnowledge]
    );

    // use multi async to handle uploading multiple files at once
    const uploadKnowledge = useMultiAsync(handleAddKnowledge);

    const fileInputRef = useRef<HTMLInputElement>(null);

    const startUpload = (files: FileList) => {
        if (!files.length) return;

        setIgnoredFiles([]);

        uploadKnowledge.execute(
            Array.from(files).map((file) => [file] as const)
        );

        if (fileInputRef.current) fileInputRef.current.value = "";
    };

    const [ignoredFiles, setIgnoredFiles] = useState<string[]>([]);

    const uploadingFiles = useMemo(
        () =>
            uploadKnowledge.states.filter(
                (state) =>
                    !state.isSuccessful &&
                    !ignoredFiles.includes(state.params[0].name)
            ),
        [ignoredFiles, uploadKnowledge.states]
    );

    useEffect(() => {
        // we can assume that the knowledge is completely ingested if all items have a status of completed or skipped
        // if that is the case, then we can block polling for updates
        const hasCompleteIngestion = getKnowledge.data?.every((item) => {
            const ingestionStatus = getIngestionStatus(item.ingestionStatus);
            return (
                ingestionStatus === IngestionStatus.Finished ||
                ingestionStatus === IngestionStatus.Skipped
            );
        });

        const hasIncompleteUpload = uploadKnowledge.states.some(
            (state) => state.isLoading
        );

        setBlockPolling(
            hasCompleteIngestion ||
                hasIncompleteUpload ||
                deleteKnowledge.isLoading
        );
    }, [uploadKnowledge.states, deleteKnowledge.isLoading, getKnowledge.data]);

    useEffect(() => {
        remoteKnowledgeSources?.forEach((source) => {
            const threadId = source.threadID;
            if (threadId && source.runID) {
                const eventSource = new EventSource(
                    ApiRoutes.threads.events(threadId).url
                );
                eventSource.onmessage = (event) => {
                    const parsedData = JSON.parse(event.data);
                    if (parsedData.prompt?.metadata?.authURL) {
                        const authURL = parsedData.prompt?.metadata?.authURL;
                        if (authURL && !localStorage.getItem(authURL)) {
                            window.open(
                                authURL,
                                "_blank",
                                "noopener,noreferrer"
                            );
                            localStorage.setItem(authURL, "true");
                            eventSource.close();
                        }
                    }
                };
                eventSource.onerror = (error) => {
                    console.error("EventSource failed:", error);
                    eventSource.close();
                };
                // Close the event source after 5 seconds to avoid connection leaks
                // At the point, the authURL should be opened and the user should have
                // enough time to authenticate
                setTimeout(() => {
                    eventSource.close();
                }, 5000);
            }
        });
    }, [remoteKnowledgeSources]);

    const handleRemoteKnowledgeSourceSync = useCallback(async () => {
        try {
            for (const source of remoteKnowledgeSources!) {
                await KnowledgeService.resyncRemoteKnowledgeSource(
                    agentId,
                    source.id
                );
            }
            setTimeout(() => {
                getRemoteKnowledgeSources.mutate();
            }, 1000);
        } catch (error) {
            console.error("Failed to resync remote knowledge source:", error);
        } finally {
            setBlockPolling(false);
        }
    }, [agentId, getRemoteKnowledgeSources, remoteKnowledgeSources]);

    return (
        <div className={cn("flex flex-col", className)}>
            <ScrollArea className="max-h-[400px]">
                {uploadingFiles.length > 0 && (
                    <div className="p-2 flex flex-wrap gap-2">
                        {uploadingFiles.map((state, index) => (
                            <FileChip
                                key={index}
                                isLoading={state.isLoading}
                                error={getErrorMessage(state.error)}
                                onAction={() =>
                                    setIgnoredFiles((prev) => [
                                        ...prev,
                                        state.params[0].name,
                                    ])
                                }
                                fileName={state.params[0].name}
                            />
                        ))}

                        <div /* spacer */ />
                    </div>
                )}

                <div className={cn("p-2 flex flex-wrap gap-2")}>
                    {knowledge.map((item) => {
                        if (item.remoteKnowledgeSourceType) {
                            return (
                                <RemoteFileItemChip
                                    key={item.fileName}
                                    url={item.fileDetails.url!}
                                    displayName={
                                        getRemoteFileDisplayName(item)!
                                    }
                                    onAction={() =>
                                        deleteKnowledge.execute(item)
                                    }
                                    statusIcon={renderStatusIcon(
                                        item.ingestionStatus
                                    )}
                                    isLoading={
                                        deleteKnowledge.isLoading &&
                                        deleteKnowledge.lastCallParams?.[0]
                                            .fileName === item.fileName
                                    }
                                    remoteKnowledgeSourceType={
                                        item.remoteKnowledgeSourceType
                                    }
                                />
                            );
                        }
                        return (
                            <FileChip
                                key={item.fileName}
                                onAction={() => deleteKnowledge.execute(item)}
                                statusIcon={renderStatusIcon(
                                    item.ingestionStatus
                                )}
                                isLoading={
                                    deleteKnowledge.isLoading &&
                                    deleteKnowledge.lastCallParams?.[0]
                                        .fileName === item.fileName
                                }
                                fileName={item.fileName}
                            />
                        );
                    })}
                </div>
            </ScrollArea>
            <footer className="flex p-2 sticky bottom-0 justify-between items-center">
                <div className="flex flex-col items-start">
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
                                                        Files ingested:{" "}
                                                        {ingestedCount}
                                                    </p>
                                                    <p>
                                                        Files queued:{" "}
                                                        {queuedCount}
                                                    </p>
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
                                            {ingestedCount !== 1
                                                ? "s"
                                                : ""}{" "}
                                            ingested
                                        </span>
                                    </>
                                );
                            }
                            return null;
                        })()}
                    </div>
                    {remoteKnowledgeSources?.map((source) => {
                        if (source.runID) {
                            return (
                                <RemoteKnowledgeSourceStatus
                                    key={source.id}
                                    source={source}
                                />
                            );
                        }
                    })}
                </div>
                <div className="flex">
                    {remoteKnowledgeSources &&
                        remoteKnowledgeSources.length > 0 && (
                            <Button
                                onClick={handleRemoteKnowledgeSourceSync}
                                className={cn("mr-2")}
                            >
                                Sync Files
                            </Button>
                        )}
                    <Button
                        variant="secondary"
                        onClick={() => setIsAddFileModalOpen(true)}
                    >
                        <PlusIcon className="w-4 h-4 mr-2" />
                        Add Knowledge
                    </Button>

                    <Input
                        ref={fileInputRef}
                        type="file"
                        className="hidden"
                        multiple
                        onChange={(e) => {
                            if (!e.target.files) return;
                            startUpload(e.target.files);
                        }}
                    />
                </div>
            </footer>
            <AddFileModal
                agentId={agentId}
                fileInputRef={fileInputRef}
                isOpen={isAddFileModalOpen}
                onOpenChange={setIsAddFileModalOpen}
                startPolling={() => {
                    setBlockPolling(false);
                }}
                remoteKnowledgeSources={remoteKnowledgeSources}
            />
        </div>
    );
}

function renderStatusIcon(status?: KnowledgeIngestionStatus) {
    if (!status || !status.status) return null;
    const [Icon, className] = ingestionIcons[status.status];

    return (
        <TooltipProvider>
            <Tooltip>
                <TooltipTrigger asChild>
                    <div>
                        {Icon === LoadingSpinner ? (
                            <LoadingSpinner
                                className={cn("w-4 h-4", className)}
                            />
                        ) : (
                            <Icon className={cn("w-4 h-4", className)} />
                        )}
                    </div>
                </TooltipTrigger>
                <TooltipContent className="whitespace-normal break-words max-w-[300px] max-h-full">
                    {getMessage(status.status, status.msg, status.error)}
                </TooltipContent>
            </Tooltip>
        </TooltipProvider>
    );
}

const ingestionIcons = {
    [IngestionStatus.Queued]: [LoadingSpinner, ""],
    [IngestionStatus.Finished]: [CheckIcon, "text-green-500"],
    [IngestionStatus.Completed]: [LoadingSpinner, ""],
    [IngestionStatus.Skipped]: [CheckIcon, "text-green-500"],
    [IngestionStatus.Starting]: [LoadingSpinner, ""],
    [IngestionStatus.Failed]: [XCircleIcon, "text-destructive"],
    [IngestionStatus.Unsupported]: [Info, "text-yellow-500"],
} as const;
