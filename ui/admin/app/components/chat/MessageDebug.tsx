import { CodeIcon } from "lucide-react";
import { useState } from "react";

import { Calls } from "~/lib/model/runs";
import { RunsService } from "~/lib/service/api/runsService";

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
    TooltipProvider,
    TooltipTrigger,
} from "~/components/ui/tooltip";

import CallFrames from "./CallFrames";

type MessageDebugProps = {
    runId: string;
    variant?: ButtonProps["variant"];
};

export function MessageDebug({ runId, variant }: MessageDebugProps) {
    const [runDebug, setRunDebug] = useState<Calls>({});

    return (
        <Dialog>
            <TooltipProvider>
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
                    <TooltipContent>
                        <p>View details</p>
                    </TooltipContent>
                </Tooltip>
            </TooltipProvider>
            <DialogContent className=" w-[50vw] max-w-[80vw] h-[80vh] flex flex-col">
                <DialogHeader>
                    <DialogTitle className="flex items-center gap-2">
                        <CodeIcon className="w-4 h-4" />
                        Run {runId}
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
