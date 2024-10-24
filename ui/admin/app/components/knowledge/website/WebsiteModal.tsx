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
    RemoteKnowledgeSource,
    RemoteKnowledgeSourceType,
} from "~/lib/model/knowledge";
import { KnowledgeService } from "~/lib/service/api/knowledgeService";

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
import RemoteKnowledgeSourceStatus from "../RemoteKnowledgeSourceStatus";
import RemoteSourceSettingModal from "../RemoteSourceSettingModal";
import AddWebsiteModal from "./AddWebsiteModal";

interface WebsiteModalProps {
    agentId: string;
    isOpen: boolean;
    onOpenChange: (open: boolean) => void;
    remoteKnowledgeSources: RemoteKnowledgeSource[];
    startPolling: () => void;
    knowledgeFiles: KnowledgeFile[];
    handleRemoteKnowledgeSourceSync: (
        sourceType: RemoteKnowledgeSourceType
    ) => void;
    ingestionError?: string;
}

export const WebsiteModal: FC<WebsiteModalProps> = ({
    agentId,
    isOpen,
    onOpenChange,
    remoteKnowledgeSources,
    startPolling,
    knowledgeFiles,
    handleRemoteKnowledgeSourceSync,
    ingestionError,
}) => {
    const [isSettingModalOpen, setIsSettingModalOpen] = useState(false);
    const [isAddWebsiteModalOpen, setIsAddWebsiteModalOpen] = useState(false);
    const [loading, setLoading] = useState(false);
    const [websites, setWebsites] = useState<string[]>([]);
    const [showTable, setShowTable] = useState<{ [key: number]: boolean }>({});

    const websiteSource = remoteKnowledgeSources.find(
        (source) => source.sourceType === "website"
    );

    useEffect(() => {
        setWebsites(websiteSource?.websiteCrawlingConfig?.urls ?? []);
    }, [websiteSource?.websiteCrawlingConfig]);

    const handleSave = async (websites: string[]) => {
        await KnowledgeService.updateRemoteKnowledgeSource(
            agentId,
            websiteSource!.id!,
            {
                ...websiteSource,
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
        for (const file of knowledgeFiles) {
            await KnowledgeService.approveKnowledgeFile(
                agentId,
                file.id!,
                true
            );
        }
        startPolling();
    };

    const hasKnowledgeFiles = knowledgeFiles.length > 0;

    return (
        <Dialog open={isOpen} onOpenChange={onOpenChange}>
            <DialogContent
                aria-describedby={undefined}
                className="data-[state=open]:animate-contentShow fixed top-[50%] left-[50%] max-h-[85vh] w-[90vw] max-w-[900px] translate-x-[-50%] translate-y-[-50%] rounded-[6px] bg-white dark:bg-secondary p-[25px] shadow-[hsl(206_22%_7%_/_35%)_0px_10px_38px_-10px,_hsl(206_22%_7%_/_20%)_0px_10px_20px_-15px] focus:outline-none"
            >
                <DialogTitle className="flex flex-row items-center text-xl font-semibold mb-4 justify-between">
                    <div className="flex flex-row items-center">
                        <Avatar className="flex-row items-center w-6 h-6 mr-2">
                            <Globe className="w-4 h-4" />
                        </Avatar>
                        Website
                    </div>
                    <div>
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
                                        onClick={() =>
                                            handleRemoteKnowledgeSourceSync(
                                                "website"
                                            )
                                        }
                                        className="mr-2"
                                        tabIndex={-1}
                                        disabled={!hasKnowledgeFiles}
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
                <ScrollArea className="max-h-[45vh] overflow-x-auto">
                    <div className="max-h-[400px] overflow-x-auto">
                        {websites.map((website, index) => (
                            <ScrollArea
                                key={index}
                                className="max-h-[400px] overflow-x-auto"
                            >
                                {/* eslint-disable-next-line */}
                                <div
                                    key={index}
                                    className="flex items-center justify-between mb-2 overflow-x-auto"
                                    onClick={() => {
                                        if (
                                            showTable[index] === undefined ||
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
                                    <Button
                                        variant="ghost"
                                        onClick={() =>
                                            handleRemoveWebsite(index)
                                        }
                                    >
                                        <Trash className="h-4 w-4 dark:text-white" />
                                    </Button>
                                    {showTable[index] ? (
                                        <ChevronUp className="h-4 w-4" />
                                    ) : (
                                        <ChevronDown className="h-4 w-4" />
                                    )}
                                </div>
                                {showTable[index] && (
                                    <div className="flex flex-col gap-2">
                                        {knowledgeFiles
                                            .filter(
                                                (item) =>
                                                    websiteSource?.state
                                                        ?.websiteCrawlingState
                                                        ?.pages?.[
                                                        item.uploadID!
                                                    ]?.parentUrl === website
                                            )
                                            .map((item) => (
                                                <RemoteFileItemChip
                                                    key={item.fileName}
                                                    file={item}
                                                    remoteKnowledgeSourceType={
                                                        item.remoteKnowledgeSourceType!
                                                    }
                                                    approveFile={async (
                                                        file,
                                                        approved
                                                    ) => {
                                                        await KnowledgeService.approveKnowledgeFile(
                                                            agentId,
                                                            file.id!,
                                                            approved
                                                        );
                                                        startPolling();
                                                    }}
                                                />
                                            ))}
                                    </div>
                                )}
                            </ScrollArea>
                        ))}
                    </div>
                </ScrollArea>

                {knowledgeFiles?.some((item) => item.approved) && (
                    <IngestionStatusComponent
                        knowledge={knowledgeFiles}
                        ingestionError={ingestionError}
                    />
                )}
                {(websiteSource?.runID || websiteSource?.error) && (
                    <RemoteKnowledgeSourceStatus source={websiteSource!} />
                )}
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
                    remoteKnowledgeSource={websiteSource!}
                />
                <AddWebsiteModal
                    agentId={agentId}
                    websiteSource={websiteSource!}
                    startPolling={startPolling}
                    isOpen={isAddWebsiteModalOpen}
                    onOpenChange={setIsAddWebsiteModalOpen}
                />
            </DialogContent>
        </Dialog>
    );
};
