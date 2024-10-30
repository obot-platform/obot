import { Plus } from "lucide-react";
import { FC, useState } from "react";

import { KnowledgeSource } from "~/lib/model/knowledge";
import { KnowledgeService } from "~/lib/service/api/knowledgeService";

import { Button } from "~/components/ui/button";
import { Dialog, DialogContent, DialogTitle } from "~/components/ui/dialog";
import { Input } from "~/components/ui/input";

interface AddWebsiteModalProps {
    agentId: string;
    knowledgeSource: KnowledgeSource | undefined;
    startPolling: () => void;
    isOpen: boolean;
    onOpenChange: (open: boolean) => void;
}

const AddWebsiteModal: FC<AddWebsiteModalProps> = ({
    agentId,
    knowledgeSource,
    startPolling,
    isOpen,
    onOpenChange,
}) => {
    const [newWebsite, setNewWebsite] = useState("");

    const handleAddWebsite = async () => {
        if (newWebsite) {
            const formattedWebsite =
                newWebsite.startsWith("http://") ||
                newWebsite.startsWith("https://")
                    ? newWebsite
                    : `https://${newWebsite}`;

            if (!knowledgeSource) {
                await KnowledgeService.createKnowledgeSource(agentId, {
                    websiteCrawlingConfig: {
                        urls: [formattedWebsite],
                    },
                });
            } else {
                await KnowledgeService.updateKnowledgeSource(
                    agentId,
                    knowledgeSource.id!,
                    {
                        websiteCrawlingConfig: {
                            urls: [
                                ...(knowledgeSource.websiteCrawlingConfig
                                    ?.urls || []),
                                formattedWebsite,
                            ],
                        },
                    }
                );
            }

            startPolling();
            setNewWebsite("");
            onOpenChange(false);
        }
    };

    return (
        <Dialog open={isOpen} onOpenChange={onOpenChange}>
            <DialogContent aria-describedby={undefined}>
                <DialogTitle className="flex flex-row items-center text-xl font-semibold mb-4 justify-between">
                    Add Website
                </DialogTitle>
                <div className="mb-4">
                    <Input
                        type="text"
                        value={newWebsite}
                        onChange={(e) => setNewWebsite(e.target.value)}
                        placeholder="Enter website URL"
                        className="w-full mb-2 dark:bg-secondary"
                    />
                    <Button onClick={handleAddWebsite} className="w-full">
                        <Plus className="mr-2 h-4 w-4" /> Add URL
                    </Button>
                </div>
            </DialogContent>
        </Dialog>
    );
};

export default AddWebsiteModal;
