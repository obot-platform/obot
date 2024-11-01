import {
    ChevronDown,
    ChevronUp,
    Globe,
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

import { LoadingSpinner } from "~/components/ui/LoadingSpinner";
import { Avatar } from "~/components/ui/avatar";
import { Button } from "~/components/ui/button";
import { Dialog, DialogContent, DialogTitle } from "~/components/ui/dialog";
import {
    Tooltip,
    TooltipContent,
    TooltipProvider,
    TooltipTrigger,
} from "~/components/ui/tooltip";

import IngestionStatusComponent from "../IngestionStatus";
import RemoteFileItemChip from "../RemoteFileItemChip";
import RemoteKnowledgeSourceStatus from "../RemoteKnowledgeSourceStatus";
import RemoteSourceSettingModal from "../RemoteSourceSettingModal";
import AddWebsiteModal from "./AddWebsiteModal";

interface WebsiteModalProps {
    agentId: string;
    isOpen: boolean;
    onOpenChange: (open: boolean) => void;
    knowledgeSource: KnowledgeSource | undefined;
    startPolling: () => void;
    files: KnowledgeFile[];
    handleRemoteKnowledgeSourceSync: (id: string) => void;
}

export const WebsiteModal: FC<WebsiteModalProps> = ({
    agentId,
    isOpen,
    onOpenChange,
    knowledgeSource,
    startPolling,
    files,
    handleRemoteKnowledgeSourceSync,
}) => {
    const [isSettingModalOpen, setIsSettingModalOpen] = useState(false);
    const [isAddWebsiteModalOpen, setIsAddWebsiteModalOpen] = useState(false);
    const [loading, setLoading] = useState(false);
    const [websites, setWebsites] = useState<string[]>([]);
    const [hideTable, setHideTable] = useState<{ [key: number]: boolean }>({});

    useEffect(() => {
        setWebsites(knowledgeSource?.websiteCrawlingConfig?.urls ?? []);
    }, [knowledgeSource?.websiteCrawlingConfig]);

    const handleSave = async (websites: string[]) => {
        await KnowledgeService.updateKnowledgeSource(
            agentId,
            knowledgeSource!.id!,
            {
                ...knowledgeSource,
                websiteCrawlingConfig: {
                    urls: websites,
                },
            }
        );
        startPolling();
    };

    const handleRemoveWebsite = async (index: number) => {
        setWebsites(websites.filter((_, i) => i !== index));
        await handleSave(websites.filter((_, i) => i !== index));
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

    const hasKnowledgeFiles = files.length > 0;

    return (
        <Dialog open={isOpen} onOpenChange={onOpenChange}>
            <DialogContent
                aria-describedby={undefined}
                className="max-h-[85vh] max-w-[85vw] bg-white dark:bg-secondary"
            >
                <DialogTitle className="flex flex-row items-center text-xl font-semibold mb-4 justify-between overflow-hidden">
                    <div className="flex flex-row items-center overflow-hidden">
                        <Avatar className="flex-row items-center w-6 h-6 mr-2">
                            <Globe className="w-4 h-4" />
                        </Avatar>
                        <span className="truncate">Website</span>
                    </div>
                    <div className="flex flex-row items-center overflow-hidden">
                        <TooltipProvider>
                            <Tooltip>
                                <TooltipTrigger asChild>
                                    <Button
                                        size="sm"
                                        variant="secondary"
                                        onClick={() =>
                                            setIsAddWebsiteModalOpen(true)
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
                                        disabled={!knowledgeSource}
                                    >
                                        <SettingsIcon className="w-4 h-4" />
                                    </Button>
                                </TooltipTrigger>
                                <TooltipContent>Settings</TooltipContent>
                            </Tooltip>
                        </TooltipProvider>
                    </div>
                </DialogTitle>
                <div className="flex flex-col gap-2 max-h-[90%] overflow-x-auto">
                    {websites.map((website, index) => (
                        <div
                            key={index}
                            className="flex flex-col gap-2 overflow-x-auto max-h-full"
                        >
                            <Button
                                key={index}
                                variant="ghost"
                                className="flex w-full items-center justify-between mb-2 overflow-x-auto"
                                onClick={() => {
                                    if (
                                        hideTable[index] === undefined ||
                                        hideTable[index] === false
                                    ) {
                                        setHideTable((prev) => ({
                                            ...prev,
                                            [index]: true,
                                        }));
                                    } else {
                                        setHideTable((prev) => ({
                                            ...prev,
                                            [index]: false,
                                        }));
                                    }
                                }}
                            >
                                <span className="flex-1 mr-2 overflow-x-auto whitespace-nowrap dark:text-white">
                                    <div className="flex items-center flex-r">
                                        <Globe className="mr-2 h-4 w-4" />
                                        <a
                                            href={website}
                                            target="_blank"
                                            rel="noopener noreferrer"
                                            className="underline"
                                        >
                                            {website}
                                        </a>
                                    </div>
                                </span>
                                <span className="mr-2 dark:text-white">
                                    {
                                        files.filter(
                                            (item) =>
                                                knowledgeSource?.syncDetails
                                                    ?.websiteCrawlingState
                                                    ?.pages?.[item.url!]
                                                    ?.parentURL === website
                                        ).length
                                    }{" "}
                                    {files.filter(
                                        (item) =>
                                            knowledgeSource?.syncDetails
                                                ?.websiteCrawlingState?.pages?.[
                                                item.url!
                                            ]?.parentURL === website
                                    ).length === 1
                                        ? "file"
                                        : "files"}
                                </span>
                                <Button
                                    variant="ghost"
                                    onClick={() => handleRemoveWebsite(index)}
                                >
                                    <Trash className="h-4 w-4 dark:text-white" />
                                </Button>
                                {hideTable[index] ? (
                                    <ChevronDown className="h-4 w-4" />
                                ) : (
                                    <ChevronUp className="h-4 w-4" />
                                )}
                            </Button>
                            {(hideTable[index] === false ||
                                hideTable[index] === undefined) && (
                                <div className="flex flex-col gap-2 max-h-[250px] overflow-y-auto">
                                    {files
                                        .filter((item) => {
                                            return (
                                                knowledgeSource?.syncDetails
                                                    ?.websiteCrawlingState
                                                    ?.pages?.[item.url!]
                                                    ?.parentURL === website
                                            );
                                        })
                                        .sort((a, b) =>
                                            a.url!.localeCompare(b.url!)
                                        )
                                        .map((item) => (
                                            <RemoteFileItemChip
                                                key={item.fileName}
                                                file={item}
                                                fileName={item.url}
                                                knowledgeSourceType={
                                                    RemoteKnowledgeSourceType.Website
                                                }
                                                approveFile={async (
                                                    file,
                                                    approved
                                                ) => {
                                                    await KnowledgeService.approveFile(
                                                        agentId,
                                                        file.id,
                                                        approved
                                                    );
                                                    startPolling();
                                                }}
                                                reingestFile={(file) => {
                                                    KnowledgeService.reingestFile(
                                                        file.agentID,
                                                        file.knowledgeSourceID,
                                                        file.id
                                                    );
                                                    startPolling();
                                                }}
                                            />
                                        ))}
                                </div>
                            )}
                        </div>
                    ))}
                </div>

                {files?.some((item) => item.approved) && (
                    <IngestionStatusComponent files={files} />
                )}
                <RemoteKnowledgeSourceStatus
                    source={knowledgeSource}
                    sourceType={RemoteKnowledgeSourceType.Website}
                />
                <RemoteSourceSettingModal
                    agentId={agentId}
                    isOpen={isSettingModalOpen}
                    onOpenChange={setIsSettingModalOpen}
                    knowledgeSource={knowledgeSource!}
                />
                <AddWebsiteModal
                    agentId={agentId}
                    knowledgeSource={knowledgeSource!}
                    startPolling={startPolling}
                    isOpen={isAddWebsiteModalOpen}
                    onOpenChange={setIsAddWebsiteModalOpen}
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
            </DialogContent>
        </Dialog>
    );
};
