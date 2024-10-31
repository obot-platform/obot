import { Globe, SettingsIcon, UploadIcon } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import useSWR, { SWRResponse } from "swr";

import {
    KnowledgeFile,
    KnowledgeFileState,
    KnowledgeSourceStatus,
} from "~/lib/model/knowledge";
import { KnowledgeService } from "~/lib/service/api/knowledgeService";
import { assetUrl } from "~/lib/utils";

import { Button } from "~/components/ui/button";

import { Avatar } from "../ui/avatar";
import FileModal from "./file/FileModal";
import { NotionModal } from "./notion/NotionModal";
import { OnedriveModal } from "./onedrive/OneDriveModal";
import { WebsiteModal } from "./website/WebsiteModal";

export default function AgentKnowledgePanel({ agentId }: { agentId: string }) {
    const [blockPollingLocalFiles, setBlockPollingLocalFiles] = useState(false);
    const [blockPollingOneDrive, setBlockPollingOneDrive] = useState(false);
    const [blockPollingNotion, setBlockPollingNotion] = useState(false);
    const [blockPollingWebsite, setBlockPollingWebsite] = useState(false);
    const [blockPollingOneDriveFiles, setBlockPollingOneDriveFiles] =
        useState(false);
    const [blockPollingNotionFiles, setBlockPollingNotionFiles] =
        useState(false);
    const [blockPollingWebsiteFiles, setBlockPollingWebsiteFiles] =
        useState(false);
    const [isAddFileModalOpen, setIsAddFileModalOpen] = useState(false);
    const [isOnedriveModalOpen, setIsOnedriveModalOpen] = useState(false);
    const [isNotionModalOpen, setIsNotionModalOpen] = useState(false);
    const [isWebsiteModalOpen, setIsWebsiteModalOpen] = useState(false);

    const getLocalFiles: SWRResponse<KnowledgeFile[], Error> = useSWR(
        KnowledgeService.getLocalKnowledgeFilesForAgent.key(agentId),
        ({ agentId }) =>
            KnowledgeService.getLocalKnowledgeFilesForAgent(agentId).then(
                (items) =>
                    items
                        .sort((a, b) => a.fileName.localeCompare(b.fileName))
                        .map(
                            (item) =>
                                ({
                                    ...item,
                                }) as KnowledgeFile
                        )
            ),
        {
            revalidateOnFocus: false,
            refreshInterval: blockPollingLocalFiles ? undefined : 1000,
        }
    );
    const localFiles = useMemo(
        () => getLocalFiles.data || [],
        [getLocalFiles.data]
    );
    const ingestedLocalFiles = useMemo(
        () =>
            localFiles.filter(
                (file) => file.state === KnowledgeFileState.Ingested
            ),
        [localFiles]
    );

    const getKnowledgeSources = useSWR(
        KnowledgeService.getKnowledgeSourcesForAgent.key(agentId),
        ({ agentId }) => KnowledgeService.getKnowledgeSourcesForAgent(agentId),
        {
            revalidateOnFocus: false,
            refreshInterval:
                blockPollingNotion &&
                blockPollingOneDrive &&
                blockPollingWebsite
                    ? undefined
                    : 1000,
        }
    );
    const knowledgeSources = useMemo(
        () => getKnowledgeSources.data || [],
        [getKnowledgeSources.data]
    );

    let notionSource = knowledgeSources.find((source) => source.notionConfig);
    let onedriveSource = knowledgeSources.find(
        (source) => source.onedriveConfig
    );
    let websiteSource = knowledgeSources.find(
        (source) => source.websiteCrawlingConfig
    );

    const getNotionFiles: SWRResponse<KnowledgeFile[], Error> = useSWR(
        KnowledgeService.getFilesForKnowledgeSource.key(
            agentId,
            notionSource?.id
        ),
        ({ agentId, sourceId }) =>
            KnowledgeService.getFilesForKnowledgeSource(agentId, sourceId).then(
                (files) =>
                    files.sort((a, b) => a.fileName.localeCompare(b.fileName))
            ),
        {
            revalidateOnFocus: false,
            refreshInterval: blockPollingNotionFiles ? undefined : 1000,
        }
    );

    const notionFiles = useMemo(
        () => getNotionFiles.data || [],
        [getNotionFiles.data]
    );
    const ingestedNotionFiles = useMemo(
        () =>
            notionFiles.filter(
                (file) => file.state === KnowledgeFileState.Ingested
            ),
        [notionFiles]
    );

    const getOnedriveFiles: SWRResponse<KnowledgeFile[], Error> = useSWR(
        KnowledgeService.getFilesForKnowledgeSource.key(
            agentId,
            onedriveSource?.id
        ),
        ({ agentId, sourceId }) =>
            KnowledgeService.getFilesForKnowledgeSource(agentId, sourceId).then(
                (files) =>
                    files.sort((a, b) => a.fileName.localeCompare(b.fileName))
            ),
        {
            revalidateOnFocus: false,
            refreshInterval: blockPollingOneDriveFiles ? undefined : 1000,
        }
    );
    const onedriveFiles = useMemo(
        () => getOnedriveFiles.data || [],
        [getOnedriveFiles.data]
    );
    const ingestedOnedriveFiles = useMemo(
        () =>
            onedriveFiles.filter(
                (file) => file.state === KnowledgeFileState.Ingested
            ),
        [onedriveFiles]
    );

    const getWebsiteFiles: SWRResponse<KnowledgeFile[], Error> = useSWR(
        KnowledgeService.getFilesForKnowledgeSource.key(
            agentId,
            websiteSource?.id
        ),
        ({ agentId, sourceId }) =>
            KnowledgeService.getFilesForKnowledgeSource(agentId, sourceId).then(
                (files) =>
                    files.sort((a, b) => a.fileName.localeCompare(b.fileName))
            ),
        {
            revalidateOnFocus: false,
            refreshInterval: blockPollingWebsiteFiles ? undefined : 1000,
        }
    );

    const websiteFiles = useMemo(
        () => getWebsiteFiles.data || [],
        [getWebsiteFiles.data]
    );
    const ingestedWebsiteFiles = useMemo(
        () =>
            websiteFiles.filter(
                (file) => file.state === KnowledgeFileState.Ingested
            ),
        [websiteFiles]
    );

    const onClickNotion = async () => {
        if (!notionSource) {
            await KnowledgeService.createKnowledgeSource(agentId, {
                notionConfig: {},
            });
            getKnowledgeSources.mutate();
            notionSource = getKnowledgeSources.data?.find(
                (source) => source.notionConfig
            );
        }
        setIsNotionModalOpen(true);
    };

    const onClickOnedrive = async () => {
        setIsOnedriveModalOpen(true);
    };

    const onClickWebsite = async () => {
        setIsWebsiteModalOpen(true);
    };

    const startPollingLocalFiles = () => {
        getLocalFiles.mutate();
        setBlockPollingLocalFiles(false);
    };

    const startPollingNotion = () => {
        getNotionFiles.mutate();
        getKnowledgeSources.mutate();
        setBlockPollingNotionFiles(false);
        setBlockPollingNotion(false);
    };

    const startPollingOneDrive = () => {
        getOnedriveFiles.mutate();
        getKnowledgeSources.mutate();
        setBlockPollingOneDriveFiles(false);
        setBlockPollingOneDrive(false);
    };

    const startPollingWebsite = () => {
        getWebsiteFiles.mutate();
        getKnowledgeSources.mutate();
        setBlockPollingWebsiteFiles(false);
        setBlockPollingWebsite(false);
    };

    const handleRemoteKnowledgeSourceSync = async (id: string) => {
        await KnowledgeService.resyncKnowledgeSource(agentId, id);
        getKnowledgeSources.mutate();
    };

    useEffect(() => {
        if (
            localFiles.every(
                (file) => file.state === KnowledgeFileState.Ingested
            )
        ) {
            setBlockPollingLocalFiles(true);
        } else {
            setBlockPollingLocalFiles(false);
        }
    }, [localFiles]);

    useEffect(() => {
        if (
            notionFiles.length > 0 &&
            notionFiles
                .filter(
                    (file) =>
                        file.state !== KnowledgeFileState.PendingApproval &&
                        file.state !== KnowledgeFileState.Unapproved
                )
                .every(
                    (file) =>
                        file.state === KnowledgeFileState.Ingested ||
                        file.state === KnowledgeFileState.Error
                ) &&
            notionSource?.state !== KnowledgeSourceStatus.Syncing
        ) {
            setBlockPollingNotionFiles(true);
        } else {
            setBlockPollingNotionFiles(false);
        }
    }, [notionFiles]);

    useEffect(() => {
        if (
            onedriveFiles.length > 0 &&
            onedriveFiles
                .filter(
                    (file) =>
                        file.state !== KnowledgeFileState.PendingApproval &&
                        file.state !== KnowledgeFileState.Unapproved
                )
                .every(
                    (file) =>
                        file.state === KnowledgeFileState.Ingested ||
                        file.state === KnowledgeFileState.Error
                ) &&
            onedriveSource?.state !== KnowledgeSourceStatus.Syncing
        ) {
            setBlockPollingOneDriveFiles(true);
        } else {
            setBlockPollingOneDriveFiles(false);
        }
    }, [onedriveFiles]);

    useEffect(() => {
        if (
            websiteFiles.length > 0 &&
            websiteFiles
                .filter(
                    (file) =>
                        file.state !== KnowledgeFileState.PendingApproval &&
                        file.state !== KnowledgeFileState.Unapproved
                )
                .every(
                    (file) =>
                        file.state === KnowledgeFileState.Ingested ||
                        file.state === KnowledgeFileState.Error
                ) &&
            websiteSource?.state !== KnowledgeSourceStatus.Syncing
        ) {
            setBlockPollingWebsiteFiles(true);
        } else {
            setBlockPollingWebsiteFiles(false);
        }
    }, [websiteFiles]);

    useEffect(() => {
        notionSource = knowledgeSources.find((source) => source.notionConfig);
        if (
            !notionSource ||
            notionSource?.state === KnowledgeSourceStatus.Synced
        ) {
            getNotionFiles.mutate();
            setBlockPollingNotion(true);
        } else {
            setBlockPollingNotion(false);
        }

        onedriveSource = knowledgeSources.find(
            (source) => source.onedriveConfig
        );
        if (
            !onedriveSource ||
            onedriveSource?.state === KnowledgeSourceStatus.Synced
        ) {
            getOnedriveFiles.mutate();
            setBlockPollingOneDrive(true);
        } else {
            setBlockPollingOneDrive(false);
        }

        websiteSource = knowledgeSources.find(
            (source) => source.websiteCrawlingConfig
        );
        if (
            !websiteSource ||
            websiteSource?.state === KnowledgeSourceStatus.Synced
        ) {
            getWebsiteFiles.mutate();
            setBlockPollingWebsite(true);
        } else {
            setBlockPollingWebsite(false);
        }
    }, [getKnowledgeSources]);

    return (
        <div className="flex flex-col gap-4 justify-center items-center">
            <div className="flex w-full items-center justify-between gap-3 rounded-md bg-background px-3 py-2 text-sm font-semibold text-gray-900 shadow-sm ring-1 ring-inset ring-muted-foreground/20 focus-visible:ring-transparent">
                <div className="flex items-center gap-2 text-foreground">
                    <UploadIcon className="h-5 w-5" />
                    <span className="text-lg font-semibold">Files</span>
                </div>
                <div className="flex flex-row items-center gap-2">
                    <div className="flex items-center gap-2">
                        {ingestedLocalFiles.length > 0 && (
                            <span className="text-sm font-medium text-gray-500">
                                {ingestedLocalFiles.length}{" "}
                                {ingestedLocalFiles.length === 1
                                    ? "file"
                                    : "files"}{" "}
                                ingested
                            </span>
                        )}
                    </div>
                    <Button
                        onClick={() => setIsAddFileModalOpen(true)}
                        className="flex items-center gap-2"
                        variant="ghost"
                    >
                        <SettingsIcon className="h-5 w-5 text-foreground" />
                    </Button>
                </div>
            </div>
            <div className="flex w-full items-center justify-between gap-3 rounded-md bg-background px-3 py-2 text-sm font-semibold text-gray-900 shadow-sm ring-1 ring-inset ring-muted-foreground/20 focus-visible:ring-transparent">
                <div className="flex items-center gap-2 text-foreground">
                    <Avatar className="h-5 w-5">
                        <img src={assetUrl("/notion.svg")} alt="Notion logo" />
                    </Avatar>
                    <span className="text-lg font-semibold">Notion</span>
                </div>
                <div className="flex flex-row items-center gap-2">
                    {ingestedNotionFiles.length > 0 && (
                        <span className="text-sm font-medium text-gray-500">
                            {ingestedNotionFiles.length}{" "}
                            {ingestedNotionFiles.length === 1
                                ? "file"
                                : "files"}{" "}
                            ingested
                        </span>
                    )}
                    <Button
                        onClick={() => onClickNotion()}
                        className="flex items-center gap-2"
                        variant="ghost"
                    >
                        <SettingsIcon className="h-5 w-5 text-foreground" />
                    </Button>
                </div>
            </div>
            <div className="flex w-full items-center justify-between gap-3 rounded-md bg-background px-3 py-2 text-sm font-semibold text-gray-900 shadow-sm ring-1 ring-inset ring-muted-foreground/20 focus-visible:ring-transparent">
                <div className="flex items-center gap-2 text-foreground">
                    <Avatar className="h-5 w-5">
                        <img
                            src={assetUrl("/onedrive.svg")}
                            alt="OneDrive logo"
                        />
                    </Avatar>
                    <span className="text-lg font-semibold">OneDrive</span>
                </div>
                <div className="flex flex-row items-center gap-2">
                    {ingestedOnedriveFiles.length > 0 && (
                        <span className="text-sm font-medium text-gray-500">
                            {ingestedOnedriveFiles.length}{" "}
                            {ingestedOnedriveFiles.length === 1
                                ? "file"
                                : "files"}{" "}
                            ingested
                        </span>
                    )}
                    <Button
                        onClick={() => onClickOnedrive()}
                        className="flex items-center gap-2"
                        variant="ghost"
                    >
                        <SettingsIcon className="h-5 w-5 text-foreground" />
                    </Button>
                </div>
            </div>
            <div className="flex w-full items-center justify-between gap-3 rounded-md bg-background px-3 py-2 text-sm font-semibold text-gray-900 shadow-sm ring-1 ring-inset ring-muted-foreground/20 focus-visible:ring-transparent">
                <div className="flex items-center gap-2 text-foreground">
                    <Globe className="h-5 w-5" />
                    <span className="text-lg font-semibold">Website</span>
                </div>
                <div className="flex flex-row items-center gap-2">
                    {ingestedWebsiteFiles.length > 0 && (
                        <span className="text-sm font-medium text-gray-500">
                            {ingestedWebsiteFiles.length}{" "}
                            {ingestedWebsiteFiles.length === 1
                                ? "file"
                                : "files"}{" "}
                            ingested
                        </span>
                    )}
                    <Button
                        onClick={() => onClickWebsite()}
                        className="flex items-center gap-2"
                        variant="ghost"
                    >
                        <SettingsIcon className="h-5 w-5 text-foreground" />
                    </Button>
                </div>
            </div>
            <FileModal
                agentId={agentId}
                isOpen={isAddFileModalOpen}
                onOpenChange={setIsAddFileModalOpen}
                startPolling={startPollingLocalFiles}
                files={localFiles}
                getLocalFiles={getLocalFiles}
            />
            <NotionModal
                agentId={agentId}
                isOpen={isNotionModalOpen}
                onOpenChange={setIsNotionModalOpen}
                knowledgeSource={notionSource}
                startPolling={startPollingNotion}
                files={notionFiles}
                handleRemoteKnowledgeSourceSync={
                    handleRemoteKnowledgeSourceSync
                }
            />
            <OnedriveModal
                agentId={agentId}
                isOpen={isOnedriveModalOpen}
                onOpenChange={setIsOnedriveModalOpen}
                knowledgeSource={onedriveSource}
                startPolling={startPollingOneDrive}
                files={onedriveFiles}
                handleRemoteKnowledgeSourceSync={
                    handleRemoteKnowledgeSourceSync
                }
            />
            <WebsiteModal
                agentId={agentId}
                isOpen={isWebsiteModalOpen}
                onOpenChange={setIsWebsiteModalOpen}
                knowledgeSource={websiteSource}
                startPolling={startPollingWebsite}
                files={websiteFiles}
                handleRemoteKnowledgeSourceSync={
                    handleRemoteKnowledgeSourceSync
                }
            />
        </div>
    );
}
