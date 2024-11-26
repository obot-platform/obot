import { Trash } from "lucide-react";

import { ToolReference } from "~/lib/model/toolReferences";
import { cn, timeSince } from "~/lib/utils";

import { TruncatedText } from "~/components/TruncatedText";
import {
    TypographyH4,
    TypographyP,
    TypographySmall,
} from "~/components/Typography";
import { ConfirmationDialog } from "~/components/composed/ConfirmationDialog";
import { ToolIcon } from "~/components/tools/ToolIcon";
import { Badge } from "~/components/ui/badge";
import { Button } from "~/components/ui/button";
import {
    Card,
    CardContent,
    CardFooter,
    CardHeader,
} from "~/components/ui/card";
import {
    Tooltip,
    TooltipContent,
    TooltipTrigger,
} from "~/components/ui/tooltip";

interface ToolCardProps {
    tool: ToolReference;
    onDelete: (id: string) => void;
}

export function ToolCard({ tool, onDelete }: ToolCardProps) {
    return (
        <Card
            className={cn("flex flex-col h-full", {
                "border-2 border-primary": tool.metadata?.bundle,
                "border-2 border-error": tool.error,
            })}
        >
            <CardHeader className="pb-2">
                <TypographyH4 className="truncate flex flex-wrap items-center gap-x-2">
                    <div className="flex flex-nowrap gap-x-2">
                        <ToolIcon
                            className="w-5 min-w-5 h-5"
                            name={tool.name}
                            icon={tool.metadata?.icon}
                        />
                        {tool.name}
                    </div>
                    {tool.error && (
                        <Tooltip>
                            <TooltipTrigger>
                                <Badge className="bg-error mb-1 pointer-events-none">
                                    Failed
                                </Badge>
                            </TooltipTrigger>
                            <TooltipContent className="max-w-xs bg-error-foreground border border-error text-foreground">
                                <TypographyP>{tool.error}</TypographyP>
                            </TooltipContent>
                        </Tooltip>
                    )}
                    {tool.metadata?.bundle && (
                        <Badge className="pointer-events-none">Bundle</Badge>
                    )}
                </TypographyH4>
            </CardHeader>
            <CardContent className="flex-grow">
                <TruncatedText
                    content={tool.reference}
                    className="max-w-full"
                />
                <TypographyP className="mt-2 text-sm text-muted-foreground line-clamp-2">
                    {tool.description || "No description available"}
                </TypographyP>
            </CardContent>
            <CardFooter className="flex justify-between items-center pt-2 h-14">
                <TypographySmall className="text-muted-foreground">
                    {timeSince(new Date(tool.created))} ago
                </TypographySmall>
                {!tool.builtin && (
                    <ConfirmationDialog
                        title="Delete Tool Reference"
                        description="Are you sure you want to delete this tool reference? This action cannot be undone."
                        onConfirm={() => onDelete(tool.id)}
                        confirmProps={{
                            variant: "destructive",
                            children: "Delete",
                        }}
                    >
                        <Button variant="ghost" size="icon">
                            <Trash className="w-5 h-5" />
                        </Button>
                    </ConfirmationDialog>
                )}
            </CardFooter>
        </Card>
    );
}
