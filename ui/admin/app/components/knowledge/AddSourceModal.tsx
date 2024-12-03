import { FC, useState } from "react";

import { KnowledgeSourceType } from "~/lib/model/knowledge";
import { KnowledgeService } from "~/lib/service/api/knowledgeService";

import KnowledgeSourceAvatar from "~/components/knowledge/KnowledgeSourceAvatar";
import { Button } from "~/components/ui/button";
import { Dialog, DialogContent, DialogTitle } from "~/components/ui/dialog";
import { Input } from "~/components/ui/input";
import { Label } from "~/components/ui/label";

interface AddSourceModalProps {
    agentId: string;
    sourceType: KnowledgeSourceType;
    startPolling: () => void;
    isOpen: boolean;
    onOpenChange: (open: boolean) => void;
    onSave: (knowledgeSourceId: string) => void;
}

const AddSourceModal: FC<AddSourceModalProps> = ({
    agentId,
    sourceType,
    startPolling,
    isOpen,
    onOpenChange,
    onSave,
}) => {
    const [newWebsite, setNewWebsite] = useState("");
    const [newLink, setNewLink] = useState("");

    const handleAddWebsite = async () => {
        if (newWebsite) {
            const trimmedWebsite = newWebsite.trim();
            const formattedWebsite =
                trimmedWebsite.startsWith("http://") ||
                trimmedWebsite.startsWith("https://")
                    ? trimmedWebsite
                    : `https://${trimmedWebsite}`;

            const res = await KnowledgeService.createKnowledgeSource(agentId, {
                websiteCrawlingConfig: {
                    urls: [formattedWebsite],
                },
            });
            onSave(res.id);
            startPolling();
            setNewWebsite("");
            onOpenChange(false);
        }
    };

    const handleAddOneDrive = async () => {
        const res = await KnowledgeService.createKnowledgeSource(agentId, {
            onedriveConfig: {
                sharedLinks: [newLink.trim()],
            },
        });
        onSave(res.id);
        setNewLink("");
        startPolling();
        onOpenChange(false);
    };

    const handleAdd = async () => {
        if (sourceType === KnowledgeSourceType.Website) {
            await handleAddWebsite();
        } else if (sourceType === KnowledgeSourceType.OneDrive) {
            await handleAddOneDrive();
        }
        startPolling();
        onOpenChange(false);
    };

    return (
        <Dialog open={isOpen} onOpenChange={onOpenChange}>
            <DialogContent aria-describedby={undefined} className="max-w-2xl">
                <DialogTitle className="flex flex-row items-center text-xl font-semibold mb-4 justify-between">
                    <div className="flex flex-row items-center">
                        <KnowledgeSourceAvatar
                            knowledgeSourceType={sourceType}
                        />
                        Add {sourceType}
                    </div>
                </DialogTitle>
                <div className="mb-4">
                    {sourceType !== KnowledgeSourceType.Notion && (
                        <div className="flex flex-col items-center justify-center mb-8">
                            <div className="w-full grid grid-cols-2 items-center justify-center gap-2">
                                <Label
                                    htmlFor="site"
                                    className="block text-sm font-medium text-center"
                                >
                                    {sourceType ===
                                        KnowledgeSourceType.Website && "Site"}
                                    {sourceType ===
                                        KnowledgeSourceType.OneDrive &&
                                        "Link URL"}
                                </Label>
                                <Input
                                    id="site"
                                    type="text"
                                    value={
                                        sourceType ===
                                        KnowledgeSourceType.Website
                                            ? newWebsite
                                            : newLink
                                    }
                                    onChange={(e) =>
                                        sourceType ===
                                        KnowledgeSourceType.Website
                                            ? setNewWebsite(e.target.value)
                                            : setNewLink(e.target.value)
                                    }
                                    placeholder={
                                        sourceType ===
                                        KnowledgeSourceType.Website
                                            ? "Enter website URL"
                                            : "Enter OneDrive folder link"
                                    }
                                    className="w-[250px] dark:bg-secondary"
                                />
                            </div>
                            {sourceType === KnowledgeSourceType.OneDrive && (
                                <p className="text-xs text-gray-500 mt-4">
                                    For instructions on obtaining a OneDrive
                                    link, see{" "}
                                    <a
                                        href="https://support.microsoft.com/en-us/office/share-onedrive-files-and-folders-9fcc2f7d-de0c-4cec-93b0-a82024800c07#ID0EDBJ=Share_with_%22Copy_link%22"
                                        target="_blank"
                                        rel="noopener noreferrer"
                                        className="underline"
                                    >
                                        this document
                                    </a>
                                    .
                                </p>
                            )}
                        </div>
                    )}
                    <div className="flex justify-end gap-2">
                        <Button
                            onClick={handleAdd}
                            className="w-1/2"
                            variant="secondary"
                        >
                            OK
                        </Button>
                        <Button
                            onClick={() => onOpenChange(false)}
                            className="w-1/2"
                            variant="secondary"
                        >
                            Cancel
                        </Button>
                    </div>
                </div>
            </DialogContent>
        </Dialog>
    );
};

export default AddSourceModal;
