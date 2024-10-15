import { RefreshCcwIcon, SettingsIcon } from "lucide-react";
import { FC, useState } from "react";

import {
    KnowledgeFile,
    RemoteKnowledgeSource,
    RemoteKnowledgeSourceType,
} from "~/lib/model/knowledge";
import { KnowledgeService } from "~/lib/service/api/knowledgeService";

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

import IngestionStatusComponent from "../IngestionStatus";

type NotionModalProps = {
    agentId: string;
    isOpen: boolean;
    onOpenChange: (open: boolean) => void;
    remoteKnowledgeSources: RemoteKnowledgeSource[];
    knowledgeFiles: KnowledgeFile[];
    startPolling: () => void;
    handleRemoteKnowledgeSourceSync: (
        knowledgeSourceType: RemoteKnowledgeSourceType
    ) => void;
};

export const NotionModal: FC<NotionModalProps> = ({
    agentId,
    isOpen,
    onOpenChange,
    remoteKnowledgeSources,
    knowledgeFiles,
    startPolling,
    handleRemoteKnowledgeSourceSync,
}) => {
    const [loading, setLoading] = useState(false);
    const [isSettingModalOpen, setIsSettingModalOpen] = useState(false);

    const notionSource = remoteKnowledgeSources.find(
        (source) => source.sourceType === "notion"
    );

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
                <DialogHeader>
                    <DialogTitle className="flex flex-row items-center text-xl font-semibold mb-4 justify-between">
                        <div className="flex flex-row items-center">
                            <Avatar className="flex-row items-center w-6 h-6 mr-2">
                                <img src="/notion.svg" alt="Notion logo" />
                            </Avatar>
                            Notion
                        </div>

                        <div>
                            <Button
                                size="sm"
                                variant="secondary"
                                onClick={() =>
                                    handleRemoteKnowledgeSourceSync("notion")
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
                </DialogHeader>
                <ScrollArea>
                    <div className="flex flex-col gap-2">
                        {knowledgeFiles.map((item) => (
                            <RemoteFileItemChip
                                key={item.fileName}
                                file={item}
                                subTitle={
                                    notionSource?.state?.notionState?.pages?.[
                                        item.uploadID!
                                    ]?.title
                                }
                                remoteKnowledgeSourceType={
                                    item.remoteKnowledgeSourceType!
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
                </ScrollArea>
                {knowledgeFiles?.some((item) => item.approved) && (
                    <IngestionStatusComponent knowledge={knowledgeFiles} />
                )}
                {notionSource?.runID && (
                    <RemoteKnowledgeSourceStatus source={notionSource!} />
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
            </DialogContent>
            <RemoteSourceSettingModal
                agentId={agentId}
                isOpen={isSettingModalOpen}
                onOpenChange={setIsSettingModalOpen}
                remoteKnowledgeSource={notionSource!}
            />
        </Dialog>
    );
};
