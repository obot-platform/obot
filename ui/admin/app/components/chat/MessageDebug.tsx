import { CodeIcon, Maximize2Icon, Minimize2Icon } from "lucide-react";
import { useState } from "react";

import { Calls } from "~/lib/model/runs";
import { RunsService } from "~/lib/service/api/runsService";

import CallFrames from "~/components/chat/CallFrames";
import { Button, ButtonProps } from "~/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogHeader,
    DialogTitle,
    DialogTrigger,
} from "~/components/ui/dialog";
import {
    Tooltip,
    TooltipContent,
    TooltipTrigger,
} from "~/components/ui/tooltip";

type MessageDebugProps = {
    runId: string;
    variant?: ButtonProps["variant"];
};

export function MessageDebug({ runId, variant }: MessageDebugProps) {
    const [runDebug, setRunDebug] = useState<Calls>({});
    const [isFullscreen, setIsFullscreen] = useState(false);

    return (
        <Dialog>
            <Tooltip>
                <TooltipTrigger asChild>
                    <DialogTrigger asChild>
                        <Button
                            size="icon"
                            variant={variant}
                            onClick={() => {
                                RunsService.getRunDebugById(runId).then(
                                    (runDebug) => {
                                        setRunDebug(runDebug.frames);
                                    }
                                );
                            }}
                        >
                            <CodeIcon className="w-4 h-4" />
                        </Button>
                    </DialogTrigger>
                </TooltipTrigger>
                <TooltipContent>View Run Info</TooltipContent>
            </Tooltip>

            <DialogContent
                className={`transition-all duration-300 ease-in-out ${
                    isFullscreen
                        ? "w-screen h-screen max-w-screen max-h-screen !rounded-none !p-6"
                        : "w-[50vw] max-w-[80vw] h-[80vh]"
                } flex flex-col`}
            >
                <DialogHeader>
                    <DialogTitle>
                        <div className="flex items-center gap-2">
                            <CodeIcon className="w-4 h-4" />
                            Run {runId}
                        </div>
                        <Button
                            variant="ghost"
                            className="text-muted-foreground absolute right-10 top-2"
                            size="icon"
                            onClick={() => setIsFullscreen(!isFullscreen)}
                        >
                            {isFullscreen ? (
                                <Minimize2Icon className="w-3.5 h-3.5" />
                            ) : (
                                <Maximize2Icon className="w-3.5 h-3.5" />
                            )}
                        </Button>
                    </DialogTitle>
                </DialogHeader>
                <DialogDescription>
                    Click below to see more information about what took place
                    behind the scenes for this particular message.
                </DialogDescription>
                {runDebug && <CallFrames calls={runDebug} />}
            </DialogContent>
        </Dialog>
    );
}
