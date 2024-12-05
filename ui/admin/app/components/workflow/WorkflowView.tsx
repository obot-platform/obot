import { Editor } from "@monaco-editor/react";
import { ReaderIcon } from "@radix-ui/react-icons";
import { dump } from "js-yaml";

import { Workflow } from "~/lib/model/workflows";

import { useTheme } from "~/components/theme";
import { Button } from "~/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogTitle,
    DialogTrigger,
} from "~/components/ui/dialog";
import {
    Tooltip,
    TooltipContent,
    TooltipTrigger,
} from "~/components/ui/tooltip";

type WorkflowViewProps = {
    workflow: Workflow;
};

export function WorkflowViewYaml({ workflow }: WorkflowViewProps) {
    const { isDark } = useTheme();

    return (
        <Tooltip>
            <Dialog>
                <DialogTrigger asChild onClick={(e) => e.stopPropagation()}>
                    <TooltipTrigger asChild>
                        <Button variant="ghost" size="icon">
                            <ReaderIcon />
                        </Button>
                    </TooltipTrigger>
                </DialogTrigger>

                <DialogContent
                    className="h-[80vh] max-w-[80vw]"
                    onClick={(e) => e.stopPropagation()}
                >
                    <DialogTitle>{workflow.name}</DialogTitle>
                    <DialogDescription>Workflow</DialogDescription>

                    <div className="h-full overflow-hidden">
                        <Editor
                            height="100%"
                            defaultLanguage="yaml"
                            defaultValue={dump(
                                JSON.parse(
                                    JSON.stringify(workflow, (_, v) =>
                                        v === null ? undefined : v
                                    )
                                )
                            )}
                            // note:(tylerslaton): There is a big long process to get the theme to be different here. We'll want to do
                            // that when an overhaul to theming is done. However, this will work for now since we'll replace this very
                            // soon anyway.
                            theme={isDark ? "hc-black" : "vs"}
                            options={{
                                padding: { top: 20 },
                                minimap: { enabled: false },
                                readOnly: true,
                                lineNumbers: "off",
                                fontSize: 14,
                                fontFamily: "monospace",
                                wordWrap: "on",
                                automaticLayout: true,
                                scrollBeyondLastLine: false,
                                cursorBlinking: "solid",
                                cursorStyle: "underline",
                                renderLineHighlight: "none",
                                domReadOnly: true,
                                selectionHighlight: false,
                                occurrencesHighlight: "off",
                                guides: {
                                    indentation: false,
                                },
                                scrollbar: {
                                    vertical: "hidden",
                                },
                                matchBrackets: "never",
                                renderWhitespace: "none",
                                colorDecorators: false,
                                links: false,
                                hover: {
                                    enabled: false,
                                },
                            }}
                        />
                    </div>
                </DialogContent>
            </Dialog>

            <TooltipContent>View YAML</TooltipContent>
        </Tooltip>
    );
}
