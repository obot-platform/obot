import { CheckIcon, Globe } from "lucide-react";

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

interface AddFileModalProps {
    agentId: string;
    isOpen: boolean;
    onOpenChange: (open: boolean) => void;
    remoteKnowledgeSources: RemoteKnowledgeSource[];
    onWebsiteModalOpen: (open: boolean) => void;
    onOneDriveModalOpen: (open: boolean) => void;
    onNotionModalOpen: (open: boolean) => void;
    getRemoteKnowledgeSources: any;
}

export const AddFileModal = ({
    agentId,
    isOpen,
    onOpenChange,
    remoteKnowledgeSources,
    onWebsiteModalOpen,
    onOneDriveModalOpen,
    onNotionModalOpen,
    getRemoteKnowledgeSources,
}: AddFileModalProps) => {
    const isNotionSourceEnabled = remoteKnowledgeSources.some(
        (source) => source.sourceType === "notion"
    );
    const isOnedriveSourceEnabled = remoteKnowledgeSources.some(
        (source) => source.sourceType === "onedrive"
    );
    const isWebsiteSourceEnabled = remoteKnowledgeSources.some(
        (source) => source.sourceType === "website"
    );
    let notionSource = remoteKnowledgeSources.find(
        (source) => source.sourceType === "notion"
    );
    let onedriveSource = remoteKnowledgeSources.find(
        (source) => source.sourceType === "onedrive"
    );
    let websiteSource = remoteKnowledgeSources.find(
        (source) => source.sourceType === "website"
    );

    const onClickNotion = async () => {
        // For notion, we need to ensure the remote knowledge source is created so that client can fetch a list of pages
        if (!notionSource) {
            await KnowledgeService.createRemoteKnowledgeSource(agentId, {
                sourceType: "notion",
                disableIngestionAfterSync: true,
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
        onOpenChange(false);
        onNotionModalOpen(true);
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
        onOpenChange(false);
        onOneDriveModalOpen(true);
    };

    const onClickWebsite = async () => {
        if (!websiteSource) {
            await KnowledgeService.createRemoteKnowledgeSource(agentId, {
                sourceType: "website",
            });
            getRemoteKnowledgeSources.mutate();
        }
        onOpenChange(false);
        onWebsiteModalOpen(true);
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
                                onClick={onClickNotion}
                                className={`flex w-full items-center justify-center mt-2 gap-3 rounded-md px-3 py-2 text-sm font-semibold shadow-sm ring-1 ring-inset ring-gray-300 focus-visible:ring-transparent hover:cursor-pointer ${
                                    isNotionSourceEnabled
                                        ? "bg-gray-200 text-gray-500 cursor-not-allowed"
                                        : "bg-white text-gray-900 hover:bg-gray-50"
                                }`}
                                disabled={isNotionSourceEnabled}
                            >
                                <Avatar className="h-5 w-5">
                                    <img
                                        src={assetUrl("/notion.svg")}
                                        alt="Notion logo"
                                    />
                                </Avatar>
                                Add Notion
                                {isNotionSourceEnabled && (
                                    <CheckIcon className="h-5 w-5" />
                                )}
                            </Button>
                            <Button
                                onClick={onClickOnedrive}
                                className={`flex w-full items-center justify-center mt-2 gap-3 rounded-md px-3 py-2 text-sm font-semibold shadow-sm ring-1 ring-inset ring-gray-300 focus-visible:ring-transparent hover:cursor-pointer ${
                                    isOnedriveSourceEnabled
                                        ? "bg-gray-200 text-gray-500 cursor-not-allowed"
                                        : "bg-white text-gray-900 hover:bg-gray-50"
                                }`}
                                disabled={isOnedriveSourceEnabled}
                            >
                                <Avatar className="h-5 w-5">
                                    <img
                                        src={assetUrl("/onedrive.svg")}
                                        alt="OneDrive logo"
                                    />
                                </Avatar>
                                <span className="text-sm font-semibold leading-6">
                                    Add OneDrive
                                </span>
                                {isOnedriveSourceEnabled && (
                                    <CheckIcon className="h-5 w-5" />
                                )}
                            </Button>
                            <Button
                                onClick={onClickWebsite}
                                className={`flex w-full items-center justify-center mt-2 gap-3 rounded-md px-3 py-2 text-sm font-semibold shadow-sm ring-1 ring-inset ring-gray-300 focus-visible:ring-transparent hover:cursor-pointer ${
                                    isWebsiteSourceEnabled
                                        ? "bg-gray-200 text-gray-500 cursor-not-allowed"
                                        : "bg-white text-gray-900 hover:bg-gray-50"
                                }`}
                                disabled={isWebsiteSourceEnabled}
                            >
                                <Globe className="h-5 w-5" />
                                <span className="text-sm font-semibold leading-6">
                                    Add Website
                                </span>
                                {isWebsiteSourceEnabled && (
                                    <CheckIcon className="h-5 w-5" />
                                )}
                            </Button>
                        </div>
                    </DialogContent>
                </DialogPortal>
            </Dialog>
        </div>
    );
};
