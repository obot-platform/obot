import { UploadIcon } from "lucide-react";
import { useCallback, useRef } from "react";
import { SWRResponse } from "swr";

import { IngestionStatus, KnowledgeFile } from "~/lib/model/knowledge";
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
import { useAsync } from "~/hooks/useAsync";
import { useMultiAsync } from "~/hooks/useMultiAsync";

import { FileChip } from "../FileItem";
import IngestionStatusComponent from "../IngestionStatus";

interface FileModalProps {
    agentId: string;
    getKnowledgeFiles: SWRResponse<KnowledgeFile[], Error>;
    isOpen: boolean;
    onOpenChange: (open: boolean) => void;
    startPolling: () => void;
    knowledge: KnowledgeFile[];
}

function FileModal({
    agentId,
    getKnowledgeFiles,
    startPolling,
    knowledge,
    isOpen,
    onOpenChange,
}: FileModalProps) {
    const fileInputRef = useRef<HTMLInputElement>(null);

    const handleAddKnowledge = useCallback(
        async (_index: number, file: File) => {
            await new Promise((resolve) => setTimeout(resolve, 1000));
            await KnowledgeService.addKnowledgeToAgent(agentId, file);

            // once added, we can immediately mutate the cache value
            // without revalidating.
            // Revalidating here would cause knowledge to be refreshed
            // for each file being uploaded, which is not desirable.
            const newItem: KnowledgeFile = {
                id: "",
                fileName: file.name,
                agentID: agentId,
                // set ingestion status to starting to ensure polling is enabled
                ingestionStatus: { status: IngestionStatus.Queued },
                fileDetails: {},
                approved: true,
            };

            // eslint-disable-next-line @typescript-eslint/no-explicit-any
            getKnowledgeFiles.mutate(
                // eslint-disable-next-line @typescript-eslint/no-explicit-any
                (prev: any) => {
                    const existingItemIndex = prev?.findIndex(
                        // eslint-disable-next-line @typescript-eslint/no-explicit-any
                        (item: any) => item.fileName === newItem.fileName
                    );
                    if (existingItemIndex !== -1 && prev) {
                        const updatedPrev = [...prev];
                        updatedPrev[existingItemIndex!] = newItem;
                        return updatedPrev;
                    } else {
                        return [newItem, ...(prev || [])];
                    }
                },
                {
                    revalidate: false,
                }
            );
            startPolling();
        },
        [agentId, getKnowledgeFiles, startPolling]
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
        await KnowledgeService.deleteKnowledgeFromAgent(agentId, item.fileName);

        // optomistic update without cache revalidation
        getKnowledgeFiles.mutate((prev: KnowledgeFile[] | undefined) =>
            prev?.filter((prevItem) => prevItem.fileName !== item.fileName)
        );
    });

    return (
        <Dialog open={isOpen} onOpenChange={onOpenChange}>
            <DialogContent
                aria-describedby={undefined}
                className="bd-secondary data-[state=open]:animate-contentShow fixed top-[50%] left-[50%] max-h-[85vh] w-[90vw] max-w-[900px] translate-x-[-50%] translate-y-[-50%] rounded-[6px] bg-white dark:bg-secondary p-[25px] shadow-[hsl(206_22%_7%_/_35%)_0px_10px_38px_-10px,_hsl(206_22%_7%_/_20%)_0px_10px_20px_-15px] focus:outline-none"
            >
                <DialogHeader className="flex flex-row justify-between items-center">
                    <DialogTitle>Manage Files</DialogTitle>
                    <Button
                        variant="secondary"
                        size="sm"
                        className="mr-2"
                        onClick={() => fileInputRef.current?.click()}
                    >
                        <UploadIcon className="upload-icon" />
                    </Button>
                </DialogHeader>
                <ScrollArea className="max-h-[800px] mt-4">
                    <div className={cn("p-2 flex flex-wrap gap-2")}>
                        {knowledge?.map((item) => (
                            <FileChip
                                key={item.fileName}
                                file={item}
                                approveFile={async (file, approved) => {
                                    await KnowledgeService.approveKnowledgeFile(
                                        agentId,
                                        file.id!,
                                        approved
                                    );
                                    startPolling();
                                }}
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
                {knowledge.some((item) => item.approved) && (
                    <IngestionStatusComponent knowledge={knowledge} />
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
