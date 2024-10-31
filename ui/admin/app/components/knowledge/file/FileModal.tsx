import { UploadIcon } from "lucide-react";
import { useCallback, useRef } from "react";

import { KnowledgeFile } from "~/lib/model/knowledge";
import { KnowledgeService } from "~/lib/service/api/knowledgeService";
import { cn } from "~/lib/utils";

import { Button } from "~/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogFooter,
    DialogHeader,
    DialogTitle,
} from "~/components/ui/dialog";
import { Input } from "~/components/ui/input";
import { ScrollArea } from "~/components/ui/scroll-area";
import {
    Tooltip,
    TooltipContent,
    TooltipProvider,
    TooltipTrigger,
} from "~/components/ui/tooltip";
import { useAsync } from "~/hooks/useAsync";
import { useMultiAsync } from "~/hooks/useMultiAsync";

import { FileChip } from "../FileItem";
import IngestionStatusComponent from "../IngestionStatus";

interface FileModalProps {
    agentId: string;
    isOpen: boolean;
    onOpenChange: (open: boolean) => void;
    startPolling: () => void;
    files: KnowledgeFile[];
}

function FileModal({
    agentId,
    startPolling,
    files,
    isOpen,
    onOpenChange,
}: FileModalProps) {
    const fileInputRef = useRef<HTMLInputElement>(null);

    const handleAddKnowledge = useCallback(
        async (_index: number, file: File) => {
            await new Promise((resolve) => setTimeout(resolve, 1000));
            await KnowledgeService.addKnowledgeFilesToAgent(agentId, file);
            startPolling();
        },
        [agentId, startPolling]
    );

    // use multi async to handle uploading multiple files at once
    const uploadKnowledge = useMultiAsync(handleAddKnowledge);

    const startUpload = (files: FileList) => {
        if (!files.length) return;

        uploadKnowledge.execute(
            Array.from(files).map((file) => [file] as const)
        );

        if (fileInputRef.current) fileInputRef.current.value = "";
    };

    const deleteKnowledge = useAsync(async (item: KnowledgeFile) => {
        await KnowledgeService.deleteKnowledgeFileFromAgent(
            agentId,
            item.fileName
        );
        startPolling();
    });

    return (
        <Dialog open={isOpen} onOpenChange={onOpenChange}>
            <DialogContent
                aria-describedby={undefined}
                className="bd-secondary data-[state=open]:animate-contentShow fixed top-[50%] left-[50%] max-h-[85vh] w-[90vw] max-w-[900px] translate-x-[-50%] translate-y-[-50%] rounded-[6px] bg-white dark:bg-secondary p-[25px] shadow-[hsl(206_22%_7%_/_35%)_0px_10px_38px_-10px,_hsl(206_22%_7%_/_20%)_0px_10px_20px_-15px] focus:outline-none"
            >
                <DialogHeader className="flex flex-row justify-between items-center">
                    <DialogTitle>Manage Files</DialogTitle>
                    <TooltipProvider>
                        <Tooltip>
                            <TooltipTrigger asChild>
                                <Button
                                    variant="secondary"
                                    size="sm"
                                    className="mr-2"
                                    onClick={() =>
                                        fileInputRef.current?.click()
                                    }
                                    tabIndex={-1}
                                >
                                    <UploadIcon className="upload-icon" />
                                </Button>
                            </TooltipTrigger>
                            <TooltipContent>Upload</TooltipContent>
                        </Tooltip>
                    </TooltipProvider>
                </DialogHeader>
                <ScrollArea className="max-h-[45vh] mt-4">
                    <div className={cn("p-2 flex flex-wrap gap-2")}>
                        {files?.map((item) => (
                            <FileChip
                                key={item.fileName}
                                file={item}
                                onAction={() => deleteKnowledge.execute(item)}
                                isLoading={
                                    deleteKnowledge.isLoading &&
                                    deleteKnowledge.lastCallParams?.[0]
                                        .fileName === item.fileName
                                }
                            />
                        ))}
                    </div>
                </ScrollArea>
                {files.some((item) => item.approved) && (
                    <IngestionStatusComponent files={files} />
                )}
                <DialogFooter className="flex justify-center">
                    <Input
                        ref={fileInputRef}
                        type="file"
                        className="hidden"
                        multiple
                        onChange={(e) => {
                            if (!e.target.files) return;
                            startUpload(e.target.files);
                        }}
                    />
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
}

export default FileModal;
