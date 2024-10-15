import {
    ChevronDown,
    ChevronUp,
    FileIcon,
    FolderIcon,
    PlusIcon,
    RefreshCcwIcon,
    SettingsIcon,
    Trash,
} from "lucide-react";
import { FC, useEffect, useState } from "react";

import {
    KnowledgeFile,
    RemoteKnowledgeSource,
    RemoteKnowledgeSourceType,
} from "~/lib/model/knowledge";
import { KnowledgeService } from "~/lib/service/api/knowledgeService";

import RemoteKnowledgeSourceStatus from "~/components/knowledge/RemoteKnowledgeSourceStatus";
import { LoadingSpinner } from "~/components/ui/LoadingSpinner";
import { Avatar } from "~/components/ui/avatar";
import { Button } from "~/components/ui/button";
import { Dialog, DialogContent, DialogTitle } from "~/components/ui/dialog";
import { ScrollArea } from "~/components/ui/scroll-area";

import IngestionStatusComponent from "../IngestionStatus";
import RemoteFileItemChip from "../RemoteFileItemChip";
import RemoteSourceSettingModal from "../RemoteSourceSettingModal";
import AddLinkModal from "./AddLinkModal";

interface OnedriveModalProps {
    agentId: string;
    isOpen: boolean;
    onOpenChange: (open: boolean) => void;
    remoteKnowledgeSources: RemoteKnowledgeSource[];
    startPolling: () => void;
    knowledgeFiles: KnowledgeFile[];
    handleRemoteKnowledgeSourceSync: (
        sourceType: RemoteKnowledgeSourceType
    ) => void;
}

export const OnedriveModal: FC<OnedriveModalProps> = ({
    agentId,
    isOpen,
    onOpenChange,
    remoteKnowledgeSources,
    startPolling,
    knowledgeFiles,
    handleRemoteKnowledgeSourceSync,
}) => {
    const [isSettingModalOpen, setIsSettingModalOpen] = useState(false);
    const [isAddLinkModalOpen, setIsAddLinkModalOpen] = useState(false);
    const [loading, setLoading] = useState(false);
    const [links, setLinks] = useState<string[]>([]);
    const [showTable, setShowTable] = useState<{ [key: number]: boolean }>({});

    const onedriveSource = remoteKnowledgeSources.find(
        (source) => source.sourceType === "onedrive"
    );

    useEffect(() => {
        setLinks(onedriveSource?.onedriveConfig?.sharedLinks || []);
    }, [onedriveSource]);

    const handleRemoveLink = (index: number) => {
        setLinks(links.filter((_, i) => i !== index));
        handleSave(links.filter((_, i) => i !== index));
    };

    const handleSave = async (links: string[]) => {
        await KnowledgeService.updateRemoteKnowledgeSource(
            agentId,
            onedriveSource!.id!,
            {
                ...onedriveSource,
                onedriveConfig: {
                    sharedLinks: links,
                },
            }
        );
        startPolling();
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

    return (
        <Dialog open={isOpen} onOpenChange={onOpenChange}>
            <DialogContent
                aria-describedby={undefined}
                className="bd-secondary data-[state=open]:animate-contentShow fixed top-[50%] left-[50%] max-h-[85vh] w-[90vw] max-w-[900px] translate-x-[-50%] translate-y-[-50%] rounded-[6px] bg-white dark:bg-secondary p-[25px] shadow-[hsl(206_22%_7%_/_35%)_0px_10px_38px_-10px,_hsl(206_22%_7%_/_20%)_0px_10px_20px_-15px] focus:outline-none"
            >
                <DialogTitle className="flex flex-row items-center text-xl font-semibold mb-4 justify-between">
                    <div className="flex flex-row items-center">
                        <Avatar className="flex-row items-center w-6 h-6 mr-2">
                            <img src="/onedrive.svg" alt="OneDrive logo" />
                        </Avatar>
                        OneDrive
                    </div>
                    <div>
                        <Button
                            size="sm"
                            variant="secondary"
                            onClick={() => setIsAddLinkModalOpen(true)}
                            className="mr-2"
                        >
                            <PlusIcon className="w-4 h-4" />
                        </Button>
                        <Button
                            size="sm"
                            variant="secondary"
                            onClick={() =>
                                handleRemoteKnowledgeSourceSync("onedrive")
                            }
                            className="mr-2"
                        >
                            <RefreshCcwIcon className="w-4 h-4" />
                        </Button>
                        <Button
                            size="sm"
                            variant="secondary"
                            onClick={() => setIsSettingModalOpen(true)}
                            className="mr-2"
                        >
                            <SettingsIcon className="w-4 h-4" />
                        </Button>
                    </div>
                </DialogTitle>
                <ScrollArea className="max-h-[800px] overflow-x-auto">
                    <div className="max-h-[400px] overflow-x-auto">
                        {links.map((link, index) => (
                            <div key={index}>
                                {/* eslint-disable-next-line */}
                                <div
                                    key={index}
                                    className="flex flex-row items-center justify-between overflow-x-auto pr-4 h-12 cursor-pointer"
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
                                    <span className="flex-1 mr-2 overflow-x-auto whitespace-nowrap pr-10 scrollbar-hide flex flex-row items-center">
                                        {onedriveSource?.state?.onedriveState
                                            ?.links?.[link]?.name ? (
                                            onedriveSource?.state?.onedriveState
                                                ?.links?.[link]?.isFolder ? (
                                                <FolderIcon className="mr-2 h-4 w-4 align-middle" />
                                            ) : (
                                                <FileIcon className="mr-2 h-4 w-4" />
                                            )
                                        ) : (
                                            <Avatar className="mr-2 h-4 w-4">
                                                <img
                                                    src="/onedrive.svg"
                                                    alt="OneDrive logo"
                                                />
                                            </Avatar>
                                        )}

                                        {onedriveSource?.state?.onedriveState
                                            ?.links?.[link]?.name ? (
                                            <a
                                                href={link}
                                                target="_blank"
                                                rel="noopener noreferrer"
                                                className="underline align-middle"
                                            >
                                                {
                                                    onedriveSource?.state
                                                        ?.onedriveState
                                                        ?.links?.[link]?.name
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
                                    {onedriveSource?.state?.onedriveState
                                        ?.links?.[link]?.isFolder &&
                                        (showTable[index] ? (
                                            <ChevronUp className="h-4 w-4" />
                                        ) : (
                                            <ChevronDown className="h-4 w-4" />
                                        ))}
                                </div>
                                {showTable[index] && (
                                    <ScrollArea className="max-h-[200px] overflow-x-auto mb-2">
                                        <div className="flex flex-col gap-2">
                                            {knowledgeFiles

                                                .filter((item) =>
                                                    onedriveSource?.state?.onedriveState?.files?.[
                                                        item.uploadID!
                                                    ]?.folderPath?.startsWith(
                                                        // eslint-disable-next-line
                                                        onedriveSource?.state
                                                            ?.onedriveState
                                                            ?.links?.[link]
                                                            ?.name!
                                                    )
                                                )
                                                .map((item) => (
                                                    <RemoteFileItemChip
                                                        key={item.fileName}
                                                        file={item}
                                                        remoteKnowledgeSourceType={
                                                            item.remoteKnowledgeSourceType!
                                                        }
                                                        subTitle={
                                                            onedriveSource
                                                                ?.state
                                                                ?.onedriveState
                                                                ?.files?.[
                                                                item.uploadID!
                                                            ]?.folderPath
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
                                    </ScrollArea>
                                )}
                            </div>
                        ))}
                        <div className="flex flex-col gap-2 mt-2">
                            {knowledgeFiles
                                .filter(
                                    (item) =>
                                        !links.some(
                                            (link) =>
                                                onedriveSource?.state?.onedriveState?.files?.[
                                                    item.uploadID!
                                                ]?.folderPath?.startsWith(
                                                    onedriveSource?.state
                                                        ?.onedriveState
                                                        ?.links?.[link]?.name ??
                                                        ""
                                                ) ?? false
                                        )
                                )
                                .map((item) => (
                                    <RemoteFileItemChip
                                        key={item.fileName}
                                        file={item}
                                        remoteKnowledgeSourceType={
                                            item.remoteKnowledgeSourceType!
                                        }
                                        subTitle={
                                            // eslint-disable-next-line
                                            onedriveSource?.state?.onedriveState
                                                ?.files?.[item.uploadID!]
                                                ?.folderPath!
                                        }
                                        approveFile={async (file, approved) => {
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
                    </div>
                </ScrollArea>
                {knowledgeFiles?.some((item) => item.approved) && (
                    <IngestionStatusComponent knowledge={knowledgeFiles} />
                )}
                {onedriveSource?.runID && (
                    <RemoteKnowledgeSourceStatus source={onedriveSource!} />
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
                        disabled={loading}
                    >
                        {loading ? (
                            <LoadingSpinner className="w-4 h-4" />
                        ) : (
                            "Add All"
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
                    remoteKnowledgeSource={onedriveSource!}
                />
                <AddLinkModal
                    agentId={agentId}
                    onedriveSource={onedriveSource!}
                    startPolling={startPolling}
                    isOpen={isAddLinkModalOpen}
                    onOpenChange={setIsAddLinkModalOpen}
                />
            </DialogContent>
        </Dialog>
    );
};
