import { LibraryIcon } from "lucide-react";

import { KnowledgeFile } from "~/lib/model/knowledge";
import { cn } from "~/lib/utils";

import { TypographyMuted } from "~/components/Typography";
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

export function KnowledgeInfo({
    knowledge,
    className,
    disabled,
}: {
    knowledge: KnowledgeFile[];
    className?: string;
    disabled?: boolean;
}) {
    return (
        <Tooltip>
            <TooltipContent>Knowledge</TooltipContent>

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

                <PopoverContent>
                    {knowledge.length > 0 ? (
                        <div className="space-y-2">
                            {knowledge.map((file) => (
                                <TypographyMuted key={file.id}>
                                    {file.fileName}
                                </TypographyMuted>
                            ))}
                        </div>
                    ) : (
                        <TypographyMuted>
                            No knowledge available
                        </TypographyMuted>
                    )}
                </PopoverContent>
            </Popover>
        </Tooltip>
    );
}
