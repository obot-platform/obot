import { LibraryIcon, PlusIcon } from "lucide-react";
import { useRef } from "react";

import { KNOWLEDGE_TOOL } from "~/lib/model/agents";
import { KnowledgeFileNamespace } from "~/lib/model/knowledge";
import { cn } from "~/lib/utils";

import { TypographyLead, TypographySmall } from "~/components/Typography";
import { useThreadAgents } from "~/components/chat/thread-helpers";
import { KnowledgeFileItem } from "~/components/knowledge/KnowledgeFileItem";
import { Button } from "~/components/ui/button";
import {
    Popover,
    PopoverContent,
    PopoverTrigger,
} from "~/components/ui/popover";
import {
    Tooltip,
    TooltipContent,
    TooltipTrigger,
} from "~/components/ui/tooltip";
import { useKnowledgeFiles } from "~/hooks/knowledge/useKnowledgeFiles";
import { useMultiAsync } from "~/hooks/useMultiAsync";

export function KnowledgeInfo({
    threadId,
    className,
}: {
    threadId: string;
    className?: string;
}) {
    const inputRef = useRef<HTMLInputElement>(null);

    const {
        localFiles: knowledge,
        addKnowledgeFile,
        deleteKnowledgeFile,
        reingestFile,
    } = useKnowledgeFiles(KnowledgeFileNamespace.Threads, threadId);

    const { data: agent } = useThreadAgents(threadId);

    const uploadKnowledge = useMultiAsync((_index: number, file: File) =>
        addKnowledgeFile(file)
    );

    const startUpload = (files: FileList) => {
        if (!files.length) return;

        uploadKnowledge.execute(Array.from(files).map((file) => [file]));

        if (inputRef.current) inputRef.current.value = "";
    };

    const disabled = !agent?.tools?.includes(KNOWLEDGE_TOOL);

    return (
        <>
            <Tooltip>
                <TooltipContent>
                    Knowledge {disabled && "(disabled for agent)"}
                </TooltipContent>

                <Popover>
                    <TooltipTrigger asChild>
                        <PopoverTrigger asChild>
                            <Button
                                size="icon-sm"
                                variant="outline"
                                className={cn("gap-2", className)}
                                startContent={<LibraryIcon />}
                                disabled={disabled}
                            />
                        </PopoverTrigger>
                    </TooltipTrigger>

                    <PopoverContent align="start" className="w-[30vw]">
                        <div className="flex justify-between items-center gap-2 mb-4">
                            <TypographyLead>Knowledge</TypographyLead>

                            <TypographySmall className="text-muted-foreground">
                                {knowledge.length || "No"} files
                            </TypographySmall>
                        </div>

                        <div className="flex flex-col gap-2">
                            <div className="space-y-2">
                                {knowledge.map((file) => (
                                    <KnowledgeFileItem
                                        key={file.id}
                                        file={file}
                                        onDelete={deleteKnowledgeFile}
                                        onReingest={(file) =>
                                            reingestFile(file.id!)
                                        }
                                    />
                                ))}
                            </div>

                            <Button
                                onClick={() => inputRef.current?.click()}
                                startContent={<PlusIcon />}
                                variant="ghost"
                                className="self-end"
                            >
                                Add Knowledge
                            </Button>
                        </div>
                    </PopoverContent>
                </Popover>
            </Tooltip>

            <input
                type="file"
                className="hidden"
                ref={inputRef}
                multiple
                onChange={(e) => {
                    if (!e.target.files) return;
                    startUpload(e.target.files);
                }}
            />
        </>
    );
}
