import type { CallFrame } from "@gptscript-ai/gptscript";
import {
    ChevronDownIcon,
    ChevronUpIcon,
    DownloadIcon,
} from "@radix-ui/react-icons";
import { useRef, useState } from "react";
import { JSONTree } from "react-json-tree";

import { Calls as CallsType } from "~/lib/model/runs";

import { Button } from "~/components/ui/button";
import { Card, CardContent } from "~/components/ui/card";
import {
    Tooltip,
    TooltipContent,
    TooltipTrigger,
} from "~/components/ui/tooltip";

const CallFrames = ({ calls }: { calls: CallsType }) => {
    const logsContainerRef = useRef<HTMLDivElement>(null);
    const [allOpen, setAllOpen] = useState(false);

    // Add download function
    const handleDownload = () => {
        const dataStr =
            "data:text/json;charset=utf-8," +
            encodeURIComponent(JSON.stringify(calls, null, 2));
        const downloadAnchorNode = document.createElement("a");
        downloadAnchorNode.setAttribute("href", dataStr);
        downloadAnchorNode.setAttribute("download", "calls.json");
        document.body.appendChild(downloadAnchorNode);
        downloadAnchorNode.click();
        downloadAnchorNode.remove();
    };

    if (!calls) return null;

    const EmptyLogs = () => (
        <Card>
            <CardContent>
                <p>Waiting for the first event from GPTScript...</p>
            </CardContent>
        </Card>
    );

    // Build tree structure
    const buildTree = (calls: CallsType) => {
        const tree: Record<string, Todo> = {};
        const rootNodes: string[] = [];

        // Sort calls by start timestamp
        const sortedCalls = Object.entries(calls).sort(
            (a, b) =>
                new Date(a[1].start).getTime() - new Date(b[1].start).getTime()
        );

        sortedCalls.forEach(([id, call]) => {
            if (
                call.tool?.name === "GPTScript Gateway Provider" ||
                call.tool?.name === "Obot"
            ) {
                return;
            }

            const parentId = call.parentID || "";
            if (!parentId) {
                rootNodes.push(id);
            } else {
                if (!tree[parentId]) {
                    tree[parentId] = [];
                }
                tree[parentId].push(id);
            }
        });

        return { tree, rootNodes };
    };

    // Render input (JSON or text)
    const renderInput = (input: Todo) => {
        if (typeof input === "string") {
            try {
                const jsonInput = JSON.parse(input);
                return (
                    <JSONTree
                        data={jsonInput}
                        theme={{
                            base00: "transparent",
                        }}
                        invertTheme={false}
                        shouldExpandNodeInitially={() => !allOpen}
                    />
                );
            } catch (_) {
                return <p className="ml-5 whitespace-pre-wrap">{input}</p>;
            }
        }
        return (
            <JSONTree
                data={input}
                theme={{
                    base00: "transparent",
                }}
                invertTheme={false}
                shouldExpandNodeInitially={() => !allOpen}
            />
        );
    };

    // Helper function to truncate and stringify input
    const truncateInput = (input: string | object): string => {
        const stringified =
            typeof input === "string" ? input : JSON.stringify(input);
        return stringified?.length > 100
            ? stringified.slice(0, 100) + "..."
            : stringified;
    };

    // Render tree recursively
    const renderTree = (nodeId: string, depth: number = 0) => {
        const call = calls[nodeId];
        const children = tree[nodeId] || [];

        return (
            <details key={nodeId} open={allOpen}>
                <summary className="cursor-pointer">
                    <Summary call={call} />
                </summary>
                <div className="ml-5 mt-2">
                    {call.tool?.source?.location &&
                        call.tool.source.location !== "inline" && (
                            <div className="mb-2 text-xs text-gray-400">
                                Source:{" "}
                                <a
                                    href={call.tool.source.location}
                                    target="_blank"
                                    rel="noopener noreferrer"
                                    className="underline hover:text-gray-300"
                                >
                                    {call.tool.source.location}
                                </a>
                            </div>
                        )}
                    <details open={allOpen}>
                        <summary className="cursor-pointer">
                            Input Message: {truncateInput(call?.input)}
                        </summary>
                        <div className="ml-5">{renderInput(call?.input)}</div>
                    </details>
                    <details open={allOpen}>
                        <summary className="cursor-pointer">
                            Output Messages
                        </summary>
                        <ul className="ml-5 list-none">
                            {call.output?.length > 0 ? (
                                call.output.flatMap((output, key) => {
                                    if (output.content) {
                                        return [
                                            <li
                                                key={`content-${key}`}
                                                className="mb-2"
                                            >
                                                <details open={allOpen}>
                                                    <summary className="cursor-pointer">
                                                        {truncateInput(
                                                            output.content
                                                        )}
                                                    </summary>
                                                    <p className="ml-5 whitespace-pre-wrap">
                                                        {output.content}
                                                    </p>
                                                </details>
                                            </li>,
                                        ];
                                    } else if (output.subCalls) {
                                        return Object.entries(
                                            output.subCalls
                                        ).map(([subCallKey, subCall]) => (
                                            <li
                                                key={`subcall-${key}-${subCallKey}`}
                                                className="mb-2"
                                            >
                                                <details open={allOpen}>
                                                    <summary className="cursor-pointer">
                                                        Tool call:{" "}
                                                        {truncateInput(
                                                            subCallKey
                                                        )}
                                                    </summary>
                                                    <p className="ml-5 whitespace-pre-wrap">
                                                        Tool Call ID:{" "}
                                                        {subCallKey}
                                                    </p>
                                                    <p className="ml-5 whitespace-pre-wrap">
                                                        Tool ID:{" "}
                                                        {subCall.toolID}
                                                    </p>
                                                    <p className="ml-5 whitespace-pre-wrap">
                                                        Input: {subCall.input}
                                                    </p>
                                                </details>
                                            </li>
                                        ));
                                    }
                                    return [];
                                })
                            ) : (
                                <li key={`no-output`}>
                                    <p className="ml-5">No output available</p>
                                </li>
                            )}
                        </ul>
                    </details>
                    {children?.length > 0 && (
                        <details open={allOpen}>
                            <summary className="cursor-pointer">
                                Subcalls
                            </summary>
                            <div className="ml-5">
                                {children.map((childId: string) =>
                                    renderTree(childId, depth + 1)
                                )}
                            </div>
                        </details>
                    )}
                    {(call.llmRequest || call.llmResponse) && (
                        <details open={allOpen}>
                            <summary className="cursor-pointer">
                                {call.llmRequest?.chatCompletion?.messages
                                    ? "LLM Request & Response"
                                    : "Tool Command and Output"}
                            </summary>
                            <div className="ml-5">
                                {call.llmRequest && (
                                    <details open={allOpen}>
                                        <summary className="cursor-pointer">
                                            {call.llmRequest?.chatCompletion
                                                ?.messages
                                                ? "Request"
                                                : "Command"}
                                        </summary>
                                        <div className="ml-5">
                                            {renderInput(call.llmRequest)}
                                        </div>
                                    </details>
                                )}
                                {call.llmResponse && (
                                    <details open={allOpen}>
                                        <summary className="cursor-pointer">
                                            {call.llmRequest?.chatCompletion
                                                ?.messages
                                                ? "Response"
                                                : "Output"}
                                        </summary>
                                        <div className="ml-5">
                                            {renderInput(call.llmResponse)}
                                        </div>
                                    </details>
                                )}
                            </div>
                        </details>
                    )}
                    {call.tool?.toolMapping && (
                        <details open={allOpen}>
                            <summary className="cursor-pointer">Tools</summary>
                            <div className="ml-5">
                                {renderToolMapping(call.tool.toolMapping)}
                            </div>
                        </details>
                    )}
                    {call.tool?.export && (
                        <details open={allOpen}>
                            <summary className="cursor-pointer">
                                Shared Tools
                            </summary>
                            <div className="ml-5">
                                {renderExports(call.tool.export)}
                            </div>
                        </details>
                    )}
                </div>
            </details>
        );
    };

    const renderToolMapping = (toolMapping: Record<string, Todo>) => {
        return Object.entries(toolMapping).map(([key, value]) => (
            <div key={key} className="mb-2">
                {value.some((item: Todo) => item.toolID !== key) ? (
                    <>
                        {key}:
                        <ul className="list-none ml-5">
                            {value.map((item: Todo, index: number) => (
                                <li key={index} className="mb-2">
                                    <p className="ml-5 whitespace-pre-wrap">
                                        {item.reference}
                                    </p>
                                    <p className="ml-5 whitespace-pre-wrap">
                                        {item.toolID}
                                    </p>
                                </li>
                            ))}
                        </ul>
                    </>
                ) : (
                    <p className="whitespace-pre-wrap">{key}</p>
                )}
            </div>
        ));
    };

    const renderExports = (exports: string[]) => {
        return (
            <ul className="list-none ml-5">
                {exports.map((item, index) => (
                    <li key={index} className="mb-2">
                        <p className="whitespace-pre-wrap">{item}</p>
                    </li>
                ))}
            </ul>
        );
    };

    const { tree, rootNodes } = buildTree(calls);
    return (
        <div
            className="h-full overflow-scroll p-4 rounded-2xl bg-foreground dark:bg-zinc-900 text-white"
            ref={logsContainerRef}
        >
            <div className="flex justify-between items-center mb-4">
                <h2 className="text-xl font-bold">Call Frames</h2>
                <div className="flex gap-2">
                    <Tooltip>
                        <TooltipTrigger asChild>
                            <Button
                                onClick={handleDownload}
                                size="icon"
                                variant="outline"
                                className="text-gray-800 dark:text-white"
                            >
                                <DownloadIcon />
                            </Button>
                        </TooltipTrigger>
                        <TooltipContent>
                            <p>Download calls data</p>
                        </TooltipContent>
                    </Tooltip>

                    <Tooltip>
                        <TooltipTrigger asChild>
                            <Button
                                onClick={() => setAllOpen(!allOpen)}
                                size="icon"
                                variant="outline"
                                className="text-gray-800 dark:text-white"
                            >
                                {allOpen ? (
                                    <ChevronUpIcon />
                                ) : (
                                    <ChevronDownIcon />
                                )}
                            </Button>
                        </TooltipTrigger>
                        <TooltipContent>
                            <p>{allOpen ? "Collapse all" : "Expand all"}</p>
                        </TooltipContent>
                    </Tooltip>
                </div>
            </div>
            {rootNodes?.length > 0 ? (
                rootNodes.map((rootId) => renderTree(rootId))
            ) : (
                <EmptyLogs />
            )}
        </div>
    );
};

const Summary = ({ call }: { call: CallFrame }) => {
    const name =
        call.tool?.name ||
        call.tool?.source?.repo ||
        call.tool?.source?.location ||
        "main";

    const startTime = new Date(call.start).toLocaleTimeString();
    const endTime = call.end
        ? new Date(call.end).toLocaleTimeString()
        : "In progress";
    const duration = call.end
        ? `${((new Date(call.end).getTime() - new Date(call.start).getTime()) / 1000).toFixed(2)}s`
        : "N/A";
    const category = call.tool?.type || "tool";

    const info = `[${category || "tool"}] [ID: ${call.id}] [${startTime} - ${endTime}, ${duration}]`;

    return (
        <h4 className="inline">
            <span className="font-bold mr-2">
                {typeof name === "string" ? name : name.Name}
            </span>
            <span className="text-sm text-gray-400">{info}</span>
        </h4>
    );
};

export default CallFrames;
