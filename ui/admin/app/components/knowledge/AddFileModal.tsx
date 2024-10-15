import { Globe, Plus } from "lucide-react";
import { RefObject, useState } from "react";

import { RemoteKnowledgeSource } from "~/lib/model/knowledge";
import { KnowledgeService } from "~/lib/service/api/knowledgeService";
import { assetUrl } from "~/lib/utils";

import { Avatar } from "~/components/ui/avatar";
import { Button } from "~/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogOverlay,
    DialogPortal,
    DialogTitle,
} from "~/components/ui/dialog";

import { NotionModal } from "./notion/NotionModal";
import { OnedriveModal } from "./onedrive/OneDriveModal";
import { WebsiteModal } from "./website/WebsiteModal";

interface AddFileModalProps {
    fileInputRef: RefObject<HTMLInputElement>;
    agentId: string;
    isOpen: boolean;
    startPolling: () => void;
    onOpenChange: (open: boolean) => void;
    remoteKnowledgeSources: RemoteKnowledgeSource[];
}

export const AddFileModal = ({
    fileInputRef,
    agentId,
    isOpen,
    startPolling,
    onOpenChange,
    remoteKnowledgeSources,
}: AddFileModalProps) => {
    const [isOnedriveModalOpen, setIsOnedriveModalOpen] = useState(false);
    const [isNotionModalOpen, setIsNotionModalOpen] = useState(false);
    const [isWebsiteModalOpen, setIsWebsiteModalOpen] = useState(false);

    const getNotionSource = async () => {
        const notionSource = remoteKnowledgeSources.find(
            (source) => source.sourceType === "notion"
        );
        return notionSource;
    };

    const onClickNotion = async () => {
        // For notion, we need to ensure the remote knowledge source is created so that client can fetch a list of pages
        const notionSource = await getNotionSource();
        if (!notionSource) {
            await KnowledgeService.createRemoteKnowledgeSource(agentId, {
                sourceType: "notion",
            });
        }
        onOpenChange(false);
        setIsNotionModalOpen(true);
        startPolling();
    };

    return (
        <div>
            <Dialog open={isOpen} onOpenChange={onOpenChange}>
                <DialogPortal>
                    <DialogOverlay className="bg-black/50 data-[state=open]:animate-overlayShow fixed inset-0" />
                    <DialogContent
                        aria-describedby={undefined}
                        className="data-[state=open]:animate-contentShow fixed top-[50%] left-[50%] max-h-[85vh] w-[90vw] max-w-[450px] translate-x-[-50%] translate-y-[-50%] rounded-[6px] bg-white p-[25px] shadow-[hsl(206_22%_7%_/_35%)_0px_10px_38px_-10px,_hsl(206_22%_7%_/_20%)_0px_10px_20px_-15px] focus:outline-none"
                    >
                        <DialogTitle />
                        <div
                            className="flex flex-col gap-2"
                            aria-describedby="add-files"
                        >
                            <Button
                                onClick={() => {
                                    fileInputRef.current?.click();
                                    onOpenChange(false);
                                }}
                                className="flex w-full items-center justify-center gap-3 rounded-md bg-white px-3 py-2 text-sm font-semibold text-gray-900 shadow-sm ring-1 ring-inset ring-gray-300 hover:bg-gray-50 focus-visible:ring-transparent hover:cursor-pointer"
                            >
                                <Plus className="h-5 w-5" />
                                <span className="text-sm font-semibold leading-6">
                                    Add Local Files
                                </span>
                            </Button>
                            <Button
                                onClick={onClickNotion}
                                className="flex w-full items-center justify-center mt-2 gap-3 rounded-md bg-white px-3 py-2 text-sm font-semibold text-gray-900 shadow-sm ring-1 ring-inset ring-gray-300 hover:bg-gray-50 focus-visible:ring-transparent hover:cursor-pointer"
                            >
                                <Avatar className="h-5 w-5">
                                    <img
                                        src={assetUrl("/notion.svg")}
                                        alt="Notion logo"
                                    />
                                </Avatar>
                                Sync From Notion
                            </Button>
                            <Button
                                onClick={() => {
                                    onOpenChange(false);
                                    setIsOnedriveModalOpen(true);
                                }}
                                className="flex w-full items-center justify-center mt-2 gap-3 rounded-md bg-white px-3 py-2 text-sm font-semibold text-gray-900 shadow-sm ring-1 ring-inset ring-gray-300 hover:bg-gray-50 focus-visible:ring-transparent hover:cursor-pointer"
                            >
                                <Avatar className="h-5 w-5">
                                    <img
                                        src={assetUrl("/onedrive.svg")}
                                        alt="OneDrive logo"
                                    />
                                </Avatar>
                                <span className="text-sm font-semibold leading-6">
                                    Sync From OneDrive
                                </span>
                            </Button>
                            <Button
                                onClick={() => {
                                    onOpenChange(false);
                                    setIsWebsiteModalOpen(true);
                                }}
                                className="flex w-full items-center justify-center mt-2 gap-3 rounded-md bg-white px-3 py-2 text-sm font-semibold text-gray-900 shadow-sm ring-1 ring-inset ring-gray-300 hover:bg-gray-50 focus-visible:ring-transparent hover:cursor-pointer"
                            >
                                <Globe className="h-5 w-5" />
                                <span className="text-sm font-semibold leading-6">
                                    Sync From Website
                                </span>
                            </Button>
                        </div>
                    </DialogContent>
                </DialogPortal>
            </Dialog>
            <NotionModal
                agentId={agentId}
                isOpen={isNotionModalOpen}
                onOpenChange={setIsNotionModalOpen}
                startPolling={startPolling}
                remoteKnowledgeSources={remoteKnowledgeSources}
            />
            <OnedriveModal
                agentId={agentId}
                isOpen={isOnedriveModalOpen}
                onOpenChange={setIsOnedriveModalOpen}
                startPolling={startPolling}
                remoteKnowledgeSources={remoteKnowledgeSources}
            />
            <WebsiteModal
                agentId={agentId}
                isOpen={isWebsiteModalOpen}
                onOpenChange={setIsWebsiteModalOpen}
                startPolling={startPolling}
                remoteKnowledgeSources={remoteKnowledgeSources}
            />
        </div>
    );
};
