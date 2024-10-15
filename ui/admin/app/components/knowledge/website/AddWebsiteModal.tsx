import { Plus } from "lucide-react";
import { FC, useState } from "react";

import { RemoteKnowledgeSource } from "~/lib/model/knowledge";
import { KnowledgeService } from "~/lib/service/api/knowledgeService";

import { Button } from "~/components/ui/button";
import { Dialog, DialogContent, DialogTitle } from "~/components/ui/dialog";
import { Input } from "~/components/ui/input";

interface AddWebsiteModalProps {
    agentId: string;
    websiteSource: RemoteKnowledgeSource;
    startPolling: () => void;
    isOpen: boolean;
    onOpenChange: (open: boolean) => void;
}

const AddWebsiteModal: FC<AddWebsiteModalProps> = ({
    agentId,
    websiteSource,
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
            await KnowledgeService.updateRemoteKnowledgeSource(
                agentId,
                websiteSource.id!,
                {
                    sourceType: "website",
                    websiteCrawlingConfig: {
                        urls: [
                            ...(websiteSource.websiteCrawlingConfig?.urls ||
                                []),
                            formattedWebsite,
                        ],
                    },
                }
            );
            startPolling();
            setNewWebsite("");
            onOpenChange(false);
        }
    };

    return (
        <Dialog open={isOpen} onOpenChange={onOpenChange}>
            <DialogContent
                aria-describedby={undefined}
                className="data-[state=open]:animate-contentShow fixed top-[50%] left-[50%] max-h-[85vh] w-[90vw] max-w-[900px] translate-x-[-50%] translate-y-[-50%] rounded-[6px] bg-white dark:bg-secondary p-[25px] shadow-[hsl(206_22%_7%_/_35%)_0px_10px_38px_-10px,_hsl(206_22%_7%_/_20%)_0px_10px_20px_-15px] focus:outline-none"
            >
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
