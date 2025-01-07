import { Trash, Upload } from "lucide-react";
import { ChangeEvent, useEffect, useRef, useState } from "react";

import { Calls } from "~/lib/model/runs";

import CallFrames from "~/components/chat/CallFrames";
import { Button } from "~/components/ui/button";
import { Card } from "~/components/ui/card";
import { ResizablePanel, ResizablePanelGroup } from "~/components/ui/resizable";

export default function Debug() {
    const [input, setInput] = useState<string>("");
    const [calls, setCalls] = useState<Calls>({});
    const [fileName, setFileName] = useState<string>("");

    const fileInputRef = useRef<HTMLInputElement>(null);

    useEffect(() => {
        try {
            setCalls(JSON.parse(input));
        } catch (e) {
            console.error(e);
        }
    }, [input]);

    const handleFileChange = (event: ChangeEvent<HTMLInputElement>) => {
        const file = event.target.files?.[0];
        if (file) {
            const reader = new FileReader();
            reader.onload = (e) => {
                const result = e.target?.result;
                if (typeof result === "string") {
                    setInput(result);
                    setFileName(file.name);
                } else {
                    console.error("File reading failed");
                }
            };
            reader.readAsText(file);
        }
    };

    const handleRemoveFile = () => {
        setInput("");
        setFileName("");
    };

    return (
        <ResizablePanelGroup direction="horizontal" className="flex-auto">
            <ResizablePanel className="p-10">
                <div className="flex flex-col items-center justify-center h-full w-full px-10">
                    {fileName ? (
                        <Card className="p-2 w-full h-full overflow-y-auto">
                            <div className="flex items-center justify-between">
                                <h3>{fileName}</h3>

                                <Button
                                    onClick={handleRemoveFile}
                                    variant="destructive"
                                    size="icon"
                                >
                                    <Trash className="w-4 h-4" />
                                </Button>
                            </div>
                            <div className="mt-4">
                                <CallFrames calls={calls} />
                            </div>
                        </Card>
                    ) : (
                        <div className="p-2 mb-10 flex items-center justify-center h-full flex-col w-1/3">
                            <div className="flex flex-col items-center justify-center space-y-4">
                                <h1>Looking to debug a run?</h1>

                                <h3 className="text-muted-foreground text-center">
                                    You&apos;re in the right place! Click the
                                    button below to upload your stack trace and
                                    you can step through all the frames behind
                                    your run.
                                </h3>
                            </div>
                            <input
                                type="file"
                                ref={fileInputRef}
                                onChange={handleFileChange}
                                className="hidden"
                                accept=".json"
                            />
                            <Button
                                onClick={() => fileInputRef.current?.click()}
                                size="lg"
                                variant="secondary"
                                className="mt-10 w-full"
                            >
                                <Upload className="w-4 h-4 mr-2" />
                                Upload File
                            </Button>
                        </div>
                    )}
                </div>
            </ResizablePanel>
        </ResizablePanelGroup>
    );
}
