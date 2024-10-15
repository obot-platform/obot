import { Plus } from "lucide-react";
import { FC, useState } from "react";

import { RemoteKnowledgeSource } from "~/lib/model/knowledge";
import { KnowledgeService } from "~/lib/service/api/knowledgeService";

import { Button } from "~/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogFooter,
    DialogHeader,
    DialogTitle,
} from "~/components/ui/dialog";
import { Input } from "~/components/ui/input";

type AddLinkModalProps = {
    agentId: string;
    onedriveSource: RemoteKnowledgeSource;
    startPolling: () => void;
    isOpen: boolean;
    onOpenChange: (open: boolean) => void;
};

const AddLinkModal: FC<AddLinkModalProps> = ({
    agentId,
    onedriveSource,
    startPolling,
    isOpen,
    onOpenChange,
}) => {
    const [newLink, setNewLink] = useState("");

    const handleSave = async () => {
        await KnowledgeService.updateRemoteKnowledgeSource(
            agentId,
            onedriveSource!.id!,
            {
                ...onedriveSource,
                onedriveConfig: {
                    sharedLinks: [
                        ...(onedriveSource.onedriveConfig?.sharedLinks || []),
                        newLink,
                    ],
                },
            }
        );
        startPolling();
        onOpenChange(false);
    };

    return (
        <Dialog open={isOpen} onOpenChange={onOpenChange}>
            <DialogContent
                aria-describedby={undefined}
                className="bd-secondary data-[state=open]:animate-contentShow fixed top-[50%] left-[50%] max-h-[85vh] w-[90vw] max-w-[400px] translate-x-[-50%] translate-y-[-50%] rounded-[6px] bg-white dark:bg-secondary p-[25px] shadow-[hsl(206_22%_7%_/_35%)_0px_10px_38px_-10px,_hsl(206_22%_7%_/_20%)_0px_10px_20px_-15px] focus:outline-none"
            >
                <DialogHeader>
                    <DialogTitle className="text-xl font-semibold mb-4">
                        Add OneDrive Link
                    </DialogTitle>
                </DialogHeader>
                <div className="mb-4">
                    <Input
                        type="text"
                        value={newLink}
                        onChange={(e) => setNewLink(e.target.value)}
                        placeholder="Enter OneDrive link"
                        className="w-full mb-4"
                    />
                    <Button onClick={handleSave} className="w-full">
                        <Plus className="mr-2 h-4 w-4" /> Add Link
                    </Button>
                </div>
                <DialogFooter>
                    <Button
                        variant="secondary"
                        onClick={() => onOpenChange(false)}
                    >
                        Close
                    </Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
};

export default AddLinkModal;
