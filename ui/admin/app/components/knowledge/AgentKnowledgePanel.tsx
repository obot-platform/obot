import { Globe, SettingsIcon, UploadIcon } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import useSWR from "swr";

import {
    IngestionStatus,
    KnowledgeFile,
    RemoteKnowledgeSourceType,
    getIngestionStatus,
} from "~/lib/model/knowledge";
import { ApiRoutes } from "~/lib/routers/apiRoutes";
import { KnowledgeService } from "~/lib/service/api/knowledgeService";

import { Button } from "~/components/ui/button";

import { Avatar } from "../ui/avatar";
import FileModal from "./file/FileModal";
import { NotionModal } from "./notion/NotionModal";
import { OnedriveModal } from "./onedrive/OneDriveModal";
import { WebsiteModal } from "./website/WebsiteModal";

export function AgentKnowledgePanel({ agentId }: { agentId: string }) {
    const [blockPolling, setBlockPolling] = useState(false);
    const [isAddFileModalOpen, setIsAddFileModalOpen] = useState(false);
    const [isOnedriveModalOpen, setIsOnedriveModalOpen] = useState(false);
    const [isNotionModalOpen, setIsNotionModalOpen] = useState(false);
    const [isWebsiteModalOpen, setIsWebsiteModalOpen] = useState(false);

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
    const knowledge = useMemo(
        () => getKnowledgeFiles.data || [],
        [getKnowledgeFiles.data]
    );

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

    let notionSource = remoteKnowledgeSources.find(
        (source) => source.sourceType === "notion"
    );
    let onedriveSource = remoteKnowledgeSources.find(
        (source) => source.sourceType === "onedrive"
    );
    const websiteSource = remoteKnowledgeSources.find(
        (source) => source.sourceType === "website"
    );

    const onClickNotion = async () => {
        // For notion, we need to ensure the remote knowledge source is created so that client can fetch a list of pages
        if (!notionSource) {
            await KnowledgeService.createRemoteKnowledgeSource(agentId, {
                sourceType: "notion",
            });
            const intervalId = setInterval(() => {
                getRemoteKnowledgeSources.mutate();
                notionSource = remoteKnowledgeSources.find(
                    (source) => source.sourceType === "notion"
                );
                if (notionSource?.runID) {
                    clearInterval(intervalId);
                }
            }, 1000);
            setTimeout(() => {
                clearInterval(intervalId);
            }, 10000);
        }
        setIsNotionModalOpen(true);
    };

    const onClickOnedrive = async () => {
        if (!onedriveSource) {
            await KnowledgeService.createRemoteKnowledgeSource(agentId, {
                sourceType: "onedrive",
            });
            const intervalId = setInterval(() => {
                getRemoteKnowledgeSources.mutate();
                onedriveSource = remoteKnowledgeSources.find(
                    (source) => source.sourceType === "onedrive"
                );
                if (onedriveSource?.runID) {
                    clearInterval(intervalId);
                }
            }, 1000);
            setTimeout(() => {
                clearInterval(intervalId);
            }, 10000);
        }
        setIsOnedriveModalOpen(true);
    };

    const onClickWebsite = async () => {
        if (!websiteSource) {
            await KnowledgeService.createRemoteKnowledgeSource(agentId, {
                sourceType: "website",
            });
            getRemoteKnowledgeSources.mutate();
        }
        setIsWebsiteModalOpen(true);
    };

    const startPolling = () => {
        getRemoteKnowledgeSources.mutate();
        getKnowledgeFiles.mutate();
        setBlockPolling(false);
    };

    const handleRemoteKnowledgeSourceSync = async (
        knowledgeSourceType: RemoteKnowledgeSourceType
    ) => {
        try {
            const source = remoteKnowledgeSources?.find(
                (source) => source.sourceType === knowledgeSourceType
            );
            if (source) {
                await KnowledgeService.resyncRemoteKnowledgeSource(
                    agentId,
                    source.id
                );
            }
            const intervalId = setInterval(() => {
                getRemoteKnowledgeSources.mutate();
                const updatedSource = remoteKnowledgeSources?.find(
                    (source) => source.sourceType === knowledgeSourceType
                );
                if (updatedSource?.runID) {
                    clearInterval(intervalId);
                }
            }, 1000);
            // this is a failsafe to clear the interval as source should be updated with runID in 10 seconds once the source is resynced
            setTimeout(() => {
                clearInterval(intervalId);
                startPolling();
            }, 10000);
        } catch (error) {
            console.error("Failed to resync remote knowledge source:", error);
        }
    };

    return (
        <div className="flex flex-col gap-4 justify-center items-center">
            <div className="flex w-full justify-between items-center px-4">
                <div className="flex items-center gap-2">
                    <UploadIcon className="h-5 w-5" />
                    <span className="text-lg font-semibold">Files</span>
                </div>
                <Button
                    onClick={() => setIsAddFileModalOpen(true)}
                    className="flex items-center gap-2"
                    variant="secondary"
                >
                    <SettingsIcon className="h-5 w-5" />
                </Button>
            </div>
            <div className="flex w-full justify-between items-center px-4">
                <div className="flex items-center gap-2">
                    <Avatar className="h-5 w-5">
                        <img src="/notion.svg" alt="Notion logo" />
                    </Avatar>
                    <span className="text-lg font-semibold">Notion</span>
                </div>
                <Button
                    onClick={() => onClickNotion()}
                    className="flex items-center gap-2"
                    variant="secondary"
                >
                    <SettingsIcon className="h-5 w-5" />
                </Button>
            </div>
            <div className="flex w-full justify-between items-center px-4">
                <div className="flex items-center gap-2">
                    <Avatar className="h-5 w-5">
                        <img src="/onedrive.svg" alt="OneDrive logo" />
                    </Avatar>
                    <span className="text-lg font-semibold">OneDrive</span>
                </div>
                <Button
                    onClick={() => onClickOnedrive()}
                    className="flex items-center gap-2"
                    variant="secondary"
                >
                    <SettingsIcon className="h-5 w-5" />
                </Button>
            </div>
            <div className="flex w-full justify-between items-center px-4">
                <div className="flex items-center gap-2">
                    <Globe className="h-5 w-5" />
                    <span className="text-lg font-semibold">Website</span>
                </div>
                <Button
                    onClick={() => onClickWebsite()}
                    className="flex items-center gap-2"
                    variant="secondary"
                >
                    <SettingsIcon className="h-5 w-5" />
                </Button>
            </div>
            <FileModal
                agentId={agentId}
                isOpen={isAddFileModalOpen}
                onOpenChange={setIsAddFileModalOpen}
                startPolling={startPolling}
                knowledge={knowledge.filter(
                    (item) => !item.remoteKnowledgeSourceType
                )}
                getKnowledgeFiles={getKnowledgeFiles}
            />
            <NotionModal
                agentId={agentId}
                isOpen={isNotionModalOpen}
                onOpenChange={setIsNotionModalOpen}
                remoteKnowledgeSources={remoteKnowledgeSources}
                startPolling={startPolling}
                knowledgeFiles={knowledge.filter(
                    (item) => item.remoteKnowledgeSourceType === "notion"
                )}
                handleRemoteKnowledgeSourceSync={
                    handleRemoteKnowledgeSourceSync
                }
            />
            <OnedriveModal
                agentId={agentId}
                isOpen={isOnedriveModalOpen}
                onOpenChange={setIsOnedriveModalOpen}
                remoteKnowledgeSources={remoteKnowledgeSources}
                startPolling={startPolling}
                knowledgeFiles={knowledge.filter(
                    (item) => item.remoteKnowledgeSourceType === "onedrive"
                )}
                handleRemoteKnowledgeSourceSync={
                    handleRemoteKnowledgeSourceSync
                }
            />
            <WebsiteModal
                agentId={agentId}
                isOpen={isWebsiteModalOpen}
                onOpenChange={setIsWebsiteModalOpen}
                remoteKnowledgeSources={remoteKnowledgeSources}
                startPolling={startPolling}
                knowledgeFiles={knowledge.filter(
                    (item) => item.remoteKnowledgeSourceType === "website"
                )}
                handleRemoteKnowledgeSourceSync={
                    handleRemoteKnowledgeSourceSync
                }
            />
        </div>
    );
}
