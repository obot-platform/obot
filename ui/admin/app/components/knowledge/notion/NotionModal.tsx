import { RefreshCcwIcon, SettingsIcon } from "lucide-react";
import { FC, useEffect, useState } from "react";

import {
    KnowledgeFile,
    KnowledgeFileState,
    KnowledgeSource,
    RemoteKnowledgeSourceType,
} from "~/lib/model/knowledge";
import { KnowledgeService } from "~/lib/service/api/knowledgeService";
import { assetUrl } from "~/lib/utils";

import RemoteFileItemChip from "~/components/knowledge/RemoteFileItemChip";
import RemoteKnowledgeSourceStatus from "~/components/knowledge/RemoteKnowledgeSourceStatus";
import RemoteSourceSettingModal from "~/components/knowledge/RemoteSourceSettingModal";
import { LoadingSpinner } from "~/components/ui/LoadingSpinner";
import { Avatar } from "~/components/ui/avatar";
import { Button } from "~/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
} from "~/components/ui/dialog";
import { ScrollArea } from "~/components/ui/scroll-area";
import {
    Tooltip,
    TooltipContent,
    TooltipProvider,
    TooltipTrigger,
} from "~/components/ui/tooltip";

import IngestionStatusComponent from "../IngestionStatus";

type NotionModalProps = {
    agentId: string;
    isOpen: boolean;
    onOpenChange: (open: boolean) => void;
    knowledgeSource: KnowledgeSource | undefined;
    files: KnowledgeFile[];
    startPolling: () => void;
    handleRemoteKnowledgeSourceSync: (id: string) => void;
};

export const NotionModal: FC<NotionModalProps> = ({
    agentId,
    isOpen,
    onOpenChange,
    knowledgeSource,
    files,
    startPolling,
    handleRemoteKnowledgeSourceSync,
}) => {
    const [loading, setLoading] = useState(false);
    const [isSettingModalOpen, setIsSettingModalOpen] = useState(false);
    const [authUrl, setAuthUrl] = useState<string>("");

    useEffect(() => {
        if (!knowledgeSource) return;

        const postLogin = async () => {
            const authUrl = await KnowledgeService.getAuthUrlForKnowledgeSource(
                agentId,
                knowledgeSource!.id!
            );
            setAuthUrl(authUrl);
        };
        postLogin();
    }, [agentId, knowledgeSource]);

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

    return (
        <Dialog open={isOpen} onOpenChange={onOpenChange}>
            <DialogContent
                aria-describedby={undefined}
                className="bd-secondary data-[state=open]:animate-contentShow fixed top-[50%] left-[50%] max-h-[85vh] w-[90vw] max-w-[900px] translate-x-[-50%] translate-y-[-50%] rounded-[6px] bg-white dark:bg-secondary p-[25px] shadow-[hsl(206_22%_7%_/_35%)_0px_10px_38px_-10px,_hsl(206_22%_7%_/_20%)_0px_10px_20px_-15px] focus:outline-none"
            >
                <DialogHeader>
                    <DialogTitle className="flex flex-row items-center text-xl font-semibold mb-4 justify-between">
                        <div className="flex flex-row items-center">
                            <Avatar className="flex-row items-center w-6 h-6 mr-2">
                                <img
                                    src={assetUrl("/notion.svg")}
                                    alt="Notion logo"
                                />
                            </Avatar>
                            Notion
                        </div>

                        <div className="flex flex-row items-center">
                            <TooltipProvider>
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
                </DialogHeader>
                {authUrl && (
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
                )}
                <ScrollArea className="max-h-[45vh] flex-grow">
                    <div className="flex flex-col gap-2">
                        {files.map((item) => (
                            <RemoteFileItemChip
                                key={item.fileName}
                                file={item}
                                fileName={item.fileName}
                                subTitle={
                                    knowledgeSource?.syncDetails?.notionState
                                        ?.pages?.[item.url!]?.folderPath
                                }
                                knowledgeSourceType={
                                    RemoteKnowledgeSourceType.Notion
                                }
                                approveFile={async (file, approved) => {
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
                                }}
                            />
                        ))}
                    </div>
                </ScrollArea>
                {files?.some(
                    (item) => item.state === KnowledgeFileState.Ingesting
                ) && <IngestionStatusComponent files={files} />}
                {!authUrl && (
                    <RemoteKnowledgeSourceStatus
                        source={knowledgeSource}
                        sourceType={RemoteKnowledgeSourceType.Notion}
                    />
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
            {knowledgeSource && (
                <>
                    <RemoteSourceSettingModal
                        agentId={agentId}
                        isOpen={isSettingModalOpen}
                        onOpenChange={setIsSettingModalOpen}
                        knowledgeSource={knowledgeSource}
                    />
                </>
            )}
        </Dialog>
    );
};
