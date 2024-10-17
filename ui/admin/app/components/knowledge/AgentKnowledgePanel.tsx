import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import useSWR from "swr";

import {
    IngestionStatus,
    KnowledgeFile,
    getIngestionStatus,
} from "~/lib/model/knowledge";
import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { KnowledgeService } from "~/lib/service/api/knowledgeService";
import { cn, getErrorMessage } from "~/lib/utils";

import { Button } from "~/components/ui/button";
import { ScrollArea } from "~/components/ui/scroll-area";
import { useMultiAsync } from "~/hooks/useMultiAsync";

import { Input } from "../ui/input";
import { FileChip } from "./FileItem";
import FileSource from "./FileSource";

export function AgentKnowledgePanel({
    agentId,
    className,
}: {
    agentId: string;
    className?: string;
}) {
    const [blockPolling, setBlockPolling] = useState(false);
    const [isAddFileModalOpen, setIsAddFileModalOpen] = useState(false);

    const getKnowledgeFiles = useSWR(
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
    const knowledge = getKnowledgeFiles.data || [];

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

            getKnowledgeFiles.mutate(
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
            setBlockPolling(false);
        },
        [agentId, getKnowledgeFiles]
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
        if (knowledge.length > 0) {
            setBlockPolling(
                remoteKnowledgeSources.every((source) => !source.runID) &&
                    knowledge.every(
                        (item) =>
                            item.ingestionStatus?.status ===
                                IngestionStatus.Finished ||
                            item.ingestionStatus?.status ===
                                IngestionStatus.Skipped
                    )
            );
        }
    }, [remoteKnowledgeSources, knowledge]);

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

                <FileSource
                    agentId={agentId}
                    remoteKnowledgeSources={remoteKnowledgeSources}
                    knowledge={knowledge}
                    fileInputRef={fileInputRef}
                    getRemoteKnowledgeSources={getRemoteKnowledgeSources}
                    getKnowledge={getKnowledgeFiles}
                    isAddFileModalOpen={isAddFileModalOpen}
                    onAddFileModalOpen={setIsAddFileModalOpen}
                    startPolling={() => setBlockPolling(false)}
                />
            </ScrollArea>
            <footer className="flex p-2 sticky bottom-0 justify-end items-center">
                <div className="flex">
                    <Button
                        variant="secondary"
                        className={cn("mr-2")}
                        onClick={() => setIsAddFileModalOpen(true)}
                    >
                        Add Sources
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
        </div>
    );
}
