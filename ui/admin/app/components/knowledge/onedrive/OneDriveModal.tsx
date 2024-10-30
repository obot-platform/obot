import {
    ChevronDown,
    ChevronUp,
    FileIcon,
    FolderIcon,
    RefreshCcwIcon,
    SettingsIcon,
    Trash,
    UploadIcon,
} from "lucide-react";
import { FC, useEffect, useState } from "react";

import {
    KnowledgeFile,
    KnowledgeFileState,
    KnowledgeSource,
    RemoteKnowledgeSourceType,
} from "~/lib/model/knowledge";
import { KnowledgeService } from "~/lib/service/api/knowledgeService";
import { assetUrl } from "~/lib/utils";

import RemoteKnowledgeSourceStatus from "~/components/knowledge/RemoteKnowledgeSourceStatus";
import { LoadingSpinner } from "~/components/ui/LoadingSpinner";
import { Avatar } from "~/components/ui/avatar";
import { Button } from "~/components/ui/button";
import { Dialog, DialogContent, DialogTitle } from "~/components/ui/dialog";
import { ScrollArea } from "~/components/ui/scroll-area";
import {
    Tooltip,
    TooltipContent,
    TooltipProvider,
    TooltipTrigger,
} from "~/components/ui/tooltip";

import IngestionStatusComponent from "../IngestionStatus";
import RemoteFileItemChip from "../RemoteFileItemChip";
import RemoteSourceSettingModal from "../RemoteSourceSettingModal";
import AddLinkModal from "./AddLinkModal";

interface OnedriveModalProps {
    agentId: string;
    isOpen: boolean;
    onOpenChange: (open: boolean) => void;
    knowledgeSource: KnowledgeSource | undefined;
    startPolling: () => void;
    files: KnowledgeFile[];
    handleRemoteKnowledgeSourceSync: (id: string) => void;
}

export const OnedriveModal: FC<OnedriveModalProps> = ({
    agentId,
    isOpen,
    onOpenChange,
    knowledgeSource,
    startPolling,
    files,
    handleRemoteKnowledgeSourceSync,
}) => {
    const [isSettingModalOpen, setIsSettingModalOpen] = useState(false);
    const [isAddLinkModalOpen, setIsAddLinkModalOpen] = useState(false);
    const [loading, setLoading] = useState(false);
    const [links, setLinks] = useState<string[]>([]);
    const [showTable, setShowTable] = useState<{ [key: number]: boolean }>({});
    const [authUrl, setAuthUrl] = useState<string>("");
    useEffect(() => {
        if (!knowledgeSource) return;

        const postLogin = async () => {
            const authUrl = await KnowledgeService.getAuthUrlForKnowledgeSource(
                agentId,
                knowledgeSource!.id!
            );
            console.log(authUrl);
            setAuthUrl(authUrl);
        };
        postLogin();
    }, [agentId, knowledgeSource]);

    useEffect(() => {
        setLinks(knowledgeSource?.onedriveConfig?.sharedLinks || []);
    }, [knowledgeSource]);

    const handleRemoveLink = (index: number) => {
        setLinks(links.filter((_, i) => i !== index));
        handleSave(links.filter((_, i) => i !== index));
    };

    const handleSave = async (links: string[]) => {
        await KnowledgeService.updateKnowledgeSource(
            agentId,
            knowledgeSource!.id!,
            {
                ...knowledgeSource!,
                onedriveConfig: {
                    sharedLinks: links,
                },
            }
        );
        startPolling();
    };

    const handleApproveAll = async () => {
        for (const file of files) {
            if (
                file.state === KnowledgeFileState.PendingApproval ||
                file.state === KnowledgeFileState.Unapproved
            ) {
                await KnowledgeService.approveFile(agentId, file.id, true);
            } else if (file.state === KnowledgeFileState.Error) {
                await KnowledgeService.reingestFile(
                    agentId,
                    knowledgeSource!.id!,
                    file.id
                );
            }
        }
        startPolling();
    };

    const getFolderAndName = (fileName: string) => {
        const parts = fileName.startsWith("/")
            ? fileName.slice(1).split("/")
            : fileName.split("/");
        if (parts.length === 1) {
            return {
                rootFolder: "",
                folder: "",
                name: parts[0],
            };
        }
        return {
            rootFolder: parts[0],
            folder: parts.slice(0, parts.length - 1).join("/"),
            name: parts[parts.length - 1],
        };
    };

    const hasKnowledgeFiles = files.length > 0;
    return (
        <Dialog open={isOpen} onOpenChange={onOpenChange}>
            <DialogContent
                aria-describedby={undefined}
                className="bd-secondary data-[state=open]:animate-contentShow fixed top-[50%] left-[50%] max-h-[85vh] w-[90vw] max-w-[900px] translate-x-[-50%] translate-y-[-50%] rounded-[6px] bg-white dark:bg-secondary p-[25px] shadow-[hsl(206_22%_7%_/_35%)_0px_10px_38px_-10px,_hsl(206_22%_7%_/_20%)_0px_10px_20px_-15px] focus:outline-none"
            >
                <DialogTitle className="flex flex-row items-center text-xl font-semibold mb-4 justify-between">
                    <div className="flex flex-row items-center">
                        <Avatar className="flex-row items-center w-6 h-6 mr-2">
                            <img
                                src={assetUrl("/onedrive.svg")}
                                alt="OneDrive logo"
                            />
                        </Avatar>
                        OneDrive
                    </div>
                    <div>
                        <TooltipProvider>
                            <Tooltip>
                                <TooltipTrigger asChild>
                                    <Button
                                        size="sm"
                                        variant="secondary"
                                        onClick={() =>
                                            setIsAddLinkModalOpen(true)
                                        }
                                        className="mr-2"
                                        tabIndex={-1}
                                    >
                                        <UploadIcon className="w-4 h-4" />
                                    </Button>
                                </TooltipTrigger>
                                <TooltipContent>Add</TooltipContent>
                            </Tooltip>
                            <Tooltip>
                                <TooltipTrigger asChild>
                                    <Button
                                        size="sm"
                                        variant="secondary"
                                        onClick={() => {
                                            if (knowledgeSource) {
                                                handleRemoteKnowledgeSourceSync(
                                                    knowledgeSource.id
                                                );
                                            }
                                        }}
                                        className="mr-2"
                                        tabIndex={-1}
                                    >
                                        <RefreshCcwIcon className="w-4 h-4" />
                                    </Button>
                                </TooltipTrigger>
                                <TooltipContent>Refresh</TooltipContent>
                            </Tooltip>
                            <Tooltip>
                                <TooltipTrigger asChild>
                                    <Button
                                        size="sm"
                                        variant="secondary"
                                        onClick={() =>
                                            setIsSettingModalOpen(true)
                                        }
                                        className="mr-2"
                                        tabIndex={-1}
                                    >
                                        <SettingsIcon className="w-4 h-4" />
                                    </Button>
                                </TooltipTrigger>
                                <TooltipContent>Settings</TooltipContent>
                            </Tooltip>
                        </TooltipProvider>
                    </div>
                </DialogTitle>

                {authUrl ? (
                    <div className="flex flex-col items-center justify-center mt-4">
                        <span className="text-sm text-gray-500">
                            Please{" "}
                            <a
                                href={authUrl}
                                target="_blank"
                                rel="noopener noreferrer"
                                className="text-gray-500 underline"
                            >
                                Sign In
                            </a>{" "}
                            to continue.
                        </span>
                    </div>
                ) : (
                    <ScrollArea className="max-h-[45vh] overflow-x-auto">
                        <div className="max-h-[400px] overflow-x-auto">
                            {links.map((link, index) => (
                                <div key={index}>
                                    <Button
                                        key={index}
                                        variant="ghost"
                                        className="flex flex-row w-full items-center justify-between overflow-x-auto pr-4 h-12 cursor-pointer"
                                        onClick={() => {
                                            if (
                                                showTable[index] ===
                                                    undefined ||
                                                showTable[index] === false
                                            ) {
                                                setShowTable((prev) => ({
                                                    ...prev,
                                                    [index]: true,
                                                }));
                                            } else {
                                                setShowTable((prev) => ({
                                                    ...prev,
                                                    [index]: false,
                                                }));
                                            }
                                        }}
                                    >
                                        <span className="flex-1 mr-2 overflow-x-auto whitespace-nowrap pr-10 scrollbar-hide flex flex-row items-center">
                                            {knowledgeSource?.syncDetails
                                                ?.onedriveState?.links?.[link]
                                                ?.name ? (
                                                knowledgeSource?.syncDetails
                                                    ?.onedriveState?.links?.[
                                                    link
                                                ]?.isFolder ? (
                                                    <FolderIcon className="mr-2 h-4 w-4 align-middle" />
                                                ) : (
                                                    <FileIcon className="mr-2 h-4 w-4" />
                                                )
                                            ) : (
                                                <Avatar className="mr-2 h-4 w-4">
                                                    <img
                                                        src={assetUrl(
                                                            "/onedrive.svg"
                                                        )}
                                                        alt="OneDrive logo"
                                                    />
                                                </Avatar>
                                            )}

                                            {knowledgeSource?.syncDetails
                                                ?.onedriveState?.links?.[link]
                                                ?.name ? (
                                                <a
                                                    href={link}
                                                    target="_blank"
                                                    rel="noopener noreferrer"
                                                    className="underline align-middle"
                                                >
                                                    {
                                                        knowledgeSource
                                                            ?.syncDetails
                                                            ?.onedriveState
                                                            ?.links?.[link]
                                                            ?.name
                                                    }
                                                </a>
                                            ) : (
                                                <span className="flex items-center">
                                                    Processing OneDrive link...
                                                    <LoadingSpinner className="ml-2 h-4 w-4" />
                                                </span>
                                            )}
                                        </span>
                                        <Button
                                            variant="ghost"
                                            onClick={(e) => {
                                                e.stopPropagation();
                                                handleRemoveLink(index);
                                            }}
                                        >
                                            <Trash className="h-4 w-4" />
                                        </Button>
                                        {knowledgeSource?.syncDetails
                                            ?.onedriveState?.links?.[link]
                                            ?.isFolder &&
                                            (showTable[index] ? (
                                                <ChevronUp className="h-4 w-4" />
                                            ) : (
                                                <ChevronDown className="h-4 w-4" />
                                            ))}
                                    </Button>
                                    {showTable[index] && (
                                        <ScrollArea className="max-h-[200px] overflow-x-auto mb-2">
                                            <div className="flex flex-col gap-2">
                                                {files
                                                    .filter((item) => {
                                                        const { rootFolder } =
                                                            getFolderAndName(
                                                                item.fileName
                                                            );
                                                        return (
                                                            rootFolder ===
                                                            knowledgeSource
                                                                ?.syncDetails
                                                                ?.onedriveState
                                                                ?.links?.[link]
                                                                ?.name
                                                        );
                                                    })
                                                    .map((item) => (
                                                        <RemoteFileItemChip
                                                            key={item.fileName}
                                                            file={item}
                                                            fileName={
                                                                getFolderAndName(
                                                                    item.fileName
                                                                ).name
                                                            }
                                                            knowledgeSourceType={
                                                                RemoteKnowledgeSourceType.OneDrive
                                                            }
                                                            subTitle={
                                                                getFolderAndName(
                                                                    item.fileName
                                                                ).folder
                                                            }
                                                            approveFile={async (
                                                                file,
                                                                approved
                                                            ) => {
                                                                await KnowledgeService.approveFile(
                                                                    agentId,
                                                                    file.id!,
                                                                    approved
                                                                );
                                                                startPolling();
                                                            }}
                                                            reingestFile={async (
                                                                file
                                                            ) => {
                                                                await KnowledgeService.reingestFile(
                                                                    file.agentID,
                                                                    file.knowledgeSourceID,
                                                                    file.id
                                                                );
                                                                startPolling();
                                                            }}
                                                        />
                                                    ))}
                                            </div>
                                        </ScrollArea>
                                    )}
                                </div>
                            ))}
                            <div className="flex flex-col gap-2 mt-2">
                                {files
                                    .filter((item) =>
                                        links.every((link) => {
                                            // If we have file state and find out that file doesn't belong to any link, then we should it as separate files as this link is pointing to a file
                                            const fileState =
                                                knowledgeSource?.syncDetails
                                                    ?.onedriveState?.files?.[
                                                    item.url!
                                                ];
                                            return (
                                                fileState &&
                                                !fileState?.folderPath?.startsWith(
                                                    knowledgeSource?.syncDetails
                                                        ?.onedriveState
                                                        ?.links?.[link]?.name ??
                                                        ""
                                                )
                                            );
                                        })
                                    )
                                    .map((item) => (
                                        <RemoteFileItemChip
                                            key={item.fileName}
                                            file={item}
                                            fileName={
                                                getFolderAndName(item.fileName)
                                                    .name
                                            }
                                            knowledgeSourceType={
                                                RemoteKnowledgeSourceType.OneDrive
                                            }
                                            subTitle={
                                                getFolderAndName(item.fileName)
                                                    .folder
                                            }
                                            approveFile={async (
                                                file,
                                                approved
                                            ) => {
                                                await KnowledgeService.approveFile(
                                                    agentId,
                                                    file.id!,
                                                    approved
                                                );
                                                startPolling();
                                            }}
                                            reingestFile={async (file) => {
                                                await KnowledgeService.reingestFile(
                                                    file.agentID,
                                                    file.knowledgeSourceID,
                                                    file.id
                                                );
                                                startPolling();
                                            }}
                                        />
                                    ))}
                            </div>
                        </div>
                    </ScrollArea>
                )}

                {files?.some((item) => item.approved) && (
                    <IngestionStatusComponent files={files} />
                )}
                <RemoteKnowledgeSourceStatus
                    source={knowledgeSource}
                    sourceType={RemoteKnowledgeSourceType.OneDrive}
                />
                <div className="mt-4 flex justify-between">
                    <Button
                        className="approve-button"
                        variant="secondary"
                        onClick={async () => {
                            setLoading(true);
                            await new Promise((resolve) =>
                                setTimeout(resolve, 1000)
                            );
                            handleApproveAll();
                            setLoading(false);
                        }}
                        disabled={loading || !hasKnowledgeFiles}
                    >
                        {loading ? (
                            <LoadingSpinner className="w-4 h-4" />
                        ) : (
                            "Ingest All"
                        )}
                    </Button>
                    <Button
                        variant="secondary"
                        onClick={() => onOpenChange(false)}
                    >
                        Close
                    </Button>
                </div>
                <RemoteSourceSettingModal
                    agentId={agentId}
                    isOpen={isSettingModalOpen}
                    onOpenChange={setIsSettingModalOpen}
                    knowledgeSource={knowledgeSource}
                />
                <AddLinkModal
                    agentId={agentId}
                    knowledgeSource={knowledgeSource}
                    startPolling={startPolling}
                    isOpen={isAddLinkModalOpen}
                    onOpenChange={setIsAddLinkModalOpen}
                />
            </DialogContent>
        </Dialog>
    );
};
