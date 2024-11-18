import { Avatar } from "@radix-ui/react-avatar";
import {
    Edit,
    EyeIcon,
    FileIcon,
    GlobeIcon,
    PlusIcon,
    RefreshCcw,
    RotateCcwIcon,
    Trash,
    UploadIcon,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import useSWR, { SWRResponse } from "swr";

import { Agent } from "~/lib/model/agents";
import {
    KnowledgeFile,
    KnowledgeFileState,
    KnowledgeSource,
    KnowledgeSourceStatus,
    KnowledgeSourceType,
    getKnowledgeSourceDisplayName,
    getKnowledgeSourceType,
} from "~/lib/model/knowledge";
import { KnowledgeService } from "~/lib/service/api/knowledgeService";
import { assetUrl } from "~/lib/utils";

import AddSourceModal from "~/components/knowledge/AddSourceModal";
import ErrorDialog from "~/components/knowledge/ErrorDialog";
import FileStatusIcon from "~/components/knowledge/FileStatusIcon";
import RemoteFileAvatar from "~/components/knowledge/KnowledgeSourceAvatar";
import KnowledgeSourceDetail from "~/components/knowledge/KnowledgeSourceDetail";
import { LoadingSpinner } from "~/components/ui/LoadingSpinner";
import { Button } from "~/components/ui/button";
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuTrigger,
} from "~/components/ui/dropdown-menu";
import { Input } from "~/components/ui/input";
import { Label } from "~/components/ui/label";
import { AutosizeTextarea } from "~/components/ui/textarea";
import {
    Tooltip,
    TooltipContent,
    TooltipProvider,
    TooltipTrigger,
} from "~/components/ui/tooltip";
import { useAsync } from "~/hooks/useAsync";
import { useMultiAsync } from "~/hooks/useMultiAsync";

type AgentKnowledgePanelProps = {
    agentId: string;
    agent: Agent;
    updateAgent: (updatedAgent: Agent) => void;
};

export default function AgentKnowledgePanel({
    agentId,
    agent,
    updateAgent,
}: AgentKnowledgePanelProps) {
    const fileInputRef = useRef<HTMLInputElement>(null);
    const [blockPollingLocalFiles, setBlockPollingLocalFiles] = useState(false);
    const [blockPollingSources, setBlockPollingSources] = useState(false);
    const [isAddSourceModalOpen, setIsAddSourceModalOpen] = useState(false);
    const [knowledgeDescription, setKnowledgeDescription] = useState(
        agent.knowledgeDescription
    );
    const [sourceType, setSourceType] = useState<KnowledgeSourceType>(
        KnowledgeSourceType.Website
    );
    const [selectedKnowledgeSourceId, setSelectedKnowledgeSourceId] = useState<
        string | undefined
    >(undefined);
    const [isEditKnowledgeSourceModalOpen, setIsEditKnowledgeSourceModalOpen] =
        useState(false);

    const [errorDialogError, setErrorDialogError] = useState("");

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
                        .filter((item) => !item.deleted)
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

    const getKnowledgeSources = useSWR(
        KnowledgeService.getKnowledgeSourcesForAgent.key(agentId),
        ({ agentId }) => KnowledgeService.getKnowledgeSourcesForAgent(agentId),
        {
            revalidateOnFocus: false,
            refreshInterval: blockPollingSources ? undefined : 1000,
        }
    );
    const knowledgeSources = useMemo(
        () => getKnowledgeSources.data || [],
        [getKnowledgeSources.data]
    );

    const handleRemoteKnowledgeSourceSync = async (id: string) => {
        const syncedSource = await KnowledgeService.resyncKnowledgeSource(
            agentId,
            id
        );
        getKnowledgeSources.mutate((prev) =>
            prev?.map((source) =>
                source.id === syncedSource.id ? syncedSource : source
            )
        );
    };

    const handleDeleteKnowledgeSource = async (id: string) => {
        await KnowledgeService.deleteKnowledgeSource(agentId, id);
        getKnowledgeSources.mutate();
    };

    useEffect(() => {
        if (
            localFiles.every(
                (file) =>
                    file.state === KnowledgeFileState.Ingested ||
                    file.state === KnowledgeFileState.Error
            )
        ) {
            setBlockPollingLocalFiles(true);
        } else {
            setBlockPollingLocalFiles(false);
        }
    }, [localFiles]);

    useEffect(() => {
        if (
            knowledgeSources.length === 0 ||
            knowledgeSources.every(
                (source) =>
                    source.state === KnowledgeSourceStatus.Synced ||
                    source.state === KnowledgeSourceStatus.Error
            )
        ) {
            setBlockPollingSources(true);
        } else {
            setBlockPollingSources(false);
        }
    }, [knowledgeSources]);

    const onSaveKnowledgeSource = (updatedSource: KnowledgeSource) => {
        getKnowledgeSources.mutate((prev) =>
            prev?.map((source) =>
                source.id === updatedSource.id ? updatedSource : source
            )
        );
    };

    //Local file upload
    const handleAddKnowledge = useCallback(
        async (_index: number, file: File) => {
            const addedFile = await KnowledgeService.addKnowledgeFilesToAgent(
                agentId,
                file
            );
            getLocalFiles.mutate((prev) =>
                prev ? [...prev, addedFile] : [addedFile]
            );
            return addedFile;
        },
        [agentId, getLocalFiles]
    );

    // use multi async to handle uploading multiple files at once
    const uploadKnowledge = useMultiAsync(handleAddKnowledge);

    const startUpload = (files: FileList) => {
        if (!files.length) return;

        uploadKnowledge.execute(
            Array.from(files).map((file) => [file] as const)
        );

        if (fileInputRef.current) fileInputRef.current.value = "";
    };

    const deleteKnowledge = useAsync(async (item: KnowledgeFile) => {
        await KnowledgeService.deleteKnowledgeFileFromAgent(
            agentId,
            item.fileName
        );
        getLocalFiles.mutate((prev) => prev?.filter((f) => f.id !== item.id));
    });

    const selectedKnowledgeSource = useMemo(() => {
        return knowledgeSources.find(
            (source) => source.id === selectedKnowledgeSourceId
        );
    }, [knowledgeSources, selectedKnowledgeSourceId]);

    return (
        <div className="flex flex-col gap-4 justify-center items-center">
            <div className="grid w-full gap-2">
                <Label htmlFor="message">Knowledge Description</Label>
                <AutosizeTextarea
                    maxHeight={200}
                    placeholder="Provide a brief description of the information contained in this knowledge base. Example: A collection of documents about the human resources policies and procedures for Acme Corporation."
                    id="message"
                    value={knowledgeDescription ?? ""}
                    onChange={(e: React.ChangeEvent<HTMLTextAreaElement>) => {
                        setKnowledgeDescription(e.target.value);
                        updateAgent({
                            ...agent,
                            knowledgeDescription: e.target.value,
                        });
                    }}
                    className="max-h-[400px]"
                />
            </div>

            <div className="flex flex-col gap-2 w-full">
                {localFiles.map((file) => (
                    <div
                        key={file.fileName}
                        className="w-full flex items-center justify-between border px-2 rounded-md"
                    >
                        <div className="flex items-center">
                            <FileIcon className="w-4 h-4 mr-2" />
                            <span>{file.fileName}</span>
                        </div>
                        <div className="flex items-center">
                            <div className="text-gray-400 text-xs mr-2">
                                {file.sizeInBytes
                                    ? file.sizeInBytes > 1000000
                                        ? (file.sizeInBytes / 1000000).toFixed(
                                              2
                                          ) + " MB"
                                        : (file.sizeInBytes / 1000).toFixed(2) +
                                          " KB"
                                    : "0 Bytes"}
                            </div>
                            <div>
                                {file.state === KnowledgeFileState.Error ? (
                                    <div className="flex items-center">
                                        <Tooltip>
                                            <TooltipTrigger asChild>
                                                <Button
                                                    variant="ghost"
                                                    size="icon"
                                                    onClick={async () => {
                                                        const reingestedFile =
                                                            await KnowledgeService.reingestFile(
                                                                agentId,
                                                                file.id!
                                                            );
                                                        getLocalFiles.mutate(
                                                            (prev) =>
                                                                prev?.map(
                                                                    (f) =>
                                                                        f.id ===
                                                                        reingestedFile.id
                                                                            ? reingestedFile
                                                                            : f
                                                                )
                                                        );
                                                        return;
                                                    }}
                                                >
                                                    <RotateCcwIcon className="w-4 h-4 text-destructive" />
                                                </Button>
                                            </TooltipTrigger>
                                            <TooltipContent>
                                                Reingest
                                            </TooltipContent>
                                        </Tooltip>

                                        <Tooltip>
                                            <TooltipTrigger asChild>
                                                <Button
                                                    variant="ghost"
                                                    size="icon"
                                                    onClick={() => {
                                                        setErrorDialogError(
                                                            file.error ?? ""
                                                        );
                                                    }}
                                                >
                                                    <EyeIcon className="w-4 h-4 text-destructive" />
                                                </Button>
                                            </TooltipTrigger>
                                            <TooltipContent>
                                                View Error
                                            </TooltipContent>
                                        </Tooltip>
                                    </div>
                                ) : (
                                    <div className="flex items-center mr-2">
                                        <FileStatusIcon file={file} />
                                    </div>
                                )}
                            </div>
                            <TooltipProvider>
                                <Tooltip>
                                    <TooltipTrigger asChild>
                                        <Button
                                            variant="ghost"
                                            size="icon"
                                            onClick={() =>
                                                deleteKnowledge.execute(file)
                                            }
                                        >
                                            <Trash className="w-4 h-4" />
                                        </Button>
                                    </TooltipTrigger>
                                    <TooltipContent>Delete</TooltipContent>
                                </Tooltip>
                            </TooltipProvider>
                        </div>
                    </div>
                ))}
                {knowledgeSources.map((source) => (
                    <div
                        key={source.id}
                        className="flex items-center justify-between w-full border px-2 rounded-md"
                    >
                        <div className="flex items-center">
                            <RemoteFileAvatar
                                knowledgeSourceType={getKnowledgeSourceType(
                                    source
                                )}
                                className="w-4 h-4"
                            />
                            <span>{getKnowledgeSourceDisplayName(source)}</span>
                        </div>
                        <div className="flex items-center">
                            <Tooltip>
                                <TooltipTrigger asChild>
                                    <Button
                                        variant="ghost"
                                        size="icon"
                                        onClick={() =>
                                            handleRemoteKnowledgeSourceSync(
                                                source.id
                                            )
                                        }
                                        onMouseEnter={() => {
                                            if (
                                                source.state ===
                                                    KnowledgeSourceStatus.Syncing ||
                                                source.state ===
                                                    KnowledgeSourceStatus.Pending
                                            ) {
                                                return;
                                            }
                                        }}
                                    >
                                        {source.state ===
                                            KnowledgeSourceStatus.Syncing ||
                                        source.state ===
                                            KnowledgeSourceStatus.Pending ? (
                                            <LoadingSpinner className="w-4 h-4" />
                                        ) : (
                                            <RefreshCcw className="w-4 h-4" />
                                        )}
                                    </Button>
                                </TooltipTrigger>
                                <TooltipContent>
                                    {source.state ===
                                        KnowledgeSourceStatus.Syncing ||
                                    source.state ===
                                        KnowledgeSourceStatus.Pending
                                        ? (source.status ?? "Syncing...")
                                        : "Sync"}
                                </TooltipContent>
                            </Tooltip>

                            <Tooltip>
                                <TooltipTrigger asChild>
                                    <Button
                                        variant="ghost"
                                        size="icon"
                                        onClick={() => {
                                            setSelectedKnowledgeSourceId(
                                                source.id
                                            );
                                            setIsEditKnowledgeSourceModalOpen(
                                                true
                                            );
                                        }}
                                    >
                                        <Edit className="w-4 h-4" />
                                    </Button>
                                </TooltipTrigger>
                                <TooltipContent>Edit</TooltipContent>
                            </Tooltip>

                            <Tooltip>
                                <TooltipTrigger asChild>
                                    <Button
                                        variant="ghost"
                                        size="icon"
                                        onClick={() =>
                                            handleDeleteKnowledgeSource(
                                                source.id
                                            )
                                        }
                                    >
                                        <Trash className="w-4 h-4" />
                                    </Button>
                                </TooltipTrigger>
                                <TooltipContent>Delete</TooltipContent>
                            </Tooltip>
                        </div>
                    </div>
                ))}
                <div className="flex justify-end w-full">
                    <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                            <Button
                                variant="ghost"
                                className="flex items-center gap-2"
                            >
                                <PlusIcon className="h-5 w-5 text-foreground" />
                                Add Knowledge
                            </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent side="top">
                            <DropdownMenuItem
                                onClick={() => fileInputRef.current?.click()}
                                className="cursor-pointer"
                            >
                                <div className="flex items-center">
                                    <UploadIcon className="w-4 h-4 mr-2" />
                                    Local Files
                                </div>
                            </DropdownMenuItem>
                            <DropdownMenuItem
                                onClick={() => {
                                    setSourceType(KnowledgeSourceType.OneDrive);
                                    setIsAddSourceModalOpen(true);
                                }}
                                className="cursor-pointer"
                            >
                                <div className="flex flex-row justify-center">
                                    <div className="flex flex-row justify-center">
                                        <div className="flex items-center justify-center">
                                            <Avatar className="h-4 w-4 mr-2">
                                                <img
                                                    src={assetUrl(
                                                        "/onedrive.svg"
                                                    )}
                                                    alt="OneDrive logo"
                                                />
                                            </Avatar>
                                        </div>
                                        <span>OneDrive</span>
                                    </div>
                                </div>
                            </DropdownMenuItem>
                            <DropdownMenuItem
                                onClick={async () => {
                                    const res =
                                        await KnowledgeService.createKnowledgeSource(
                                            agentId,
                                            {
                                                notionConfig: {},
                                            }
                                        );
                                    getKnowledgeSources.mutate();
                                    setSelectedKnowledgeSourceId(res.id);
                                    setIsEditKnowledgeSourceModalOpen(true);
                                }}
                                className="cursor-pointer"
                                disabled={knowledgeSources.some(
                                    (source) =>
                                        getKnowledgeSourceType(source) ===
                                        KnowledgeSourceType.Notion
                                )}
                            >
                                <div className="flex flex-row justify-center">
                                    <Avatar className="h-4 w-4 mr-2">
                                        <img
                                            src={assetUrl("/notion.svg")}
                                            alt="Notion logo"
                                        />
                                    </Avatar>
                                    Notion
                                </div>
                            </DropdownMenuItem>
                            <DropdownMenuItem
                                onClick={() => {
                                    setSourceType(KnowledgeSourceType.Website);
                                    setIsAddSourceModalOpen(true);
                                }}
                                className="cursor-pointer"
                            >
                                <div className="flex justify-center">
                                    <GlobeIcon className="w-4 h-4 mr-2" />
                                    Website
                                </div>
                            </DropdownMenuItem>
                        </DropdownMenuContent>
                    </DropdownMenu>
                </div>
            </div>

            <AddSourceModal
                agentId={agentId}
                isOpen={isAddSourceModalOpen}
                sourceType={sourceType}
                onOpenChange={setIsAddSourceModalOpen}
                startPolling={() => {
                    getKnowledgeSources.mutate();
                }}
                onSave={(knowledgeSourceId) => {
                    setSelectedKnowledgeSourceId(knowledgeSourceId);
                    setIsEditKnowledgeSourceModalOpen(true);
                }}
            />
            <ErrorDialog
                error={errorDialogError}
                isOpen={errorDialogError !== ""}
                onClose={() => setErrorDialogError("")}
            />
            {selectedKnowledgeSourceId && selectedKnowledgeSource && (
                <KnowledgeSourceDetail
                    agentId={agentId}
                    knowledgeSource={selectedKnowledgeSource}
                    isOpen={isEditKnowledgeSourceModalOpen}
                    onOpenChange={setIsEditKnowledgeSourceModalOpen}
                    onSyncNow={() =>
                        handleRemoteKnowledgeSourceSync(
                            selectedKnowledgeSourceId
                        )
                    }
                    onDelete={() =>
                        handleDeleteKnowledgeSource(selectedKnowledgeSourceId)
                    }
                    onSave={onSaveKnowledgeSource}
                />
            )}
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
    );
}
