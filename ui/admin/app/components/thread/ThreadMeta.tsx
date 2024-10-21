import { Link } from "@remix-run/react";
import { EditIcon, FileIcon, FilesIcon } from "lucide-react";

import { Agent } from "~/lib/model/agents";
import { KnowledgeFile } from "~/lib/model/knowledge";
import { runStateToBadgeColor } from "~/lib/model/runs";
import { Thread } from "~/lib/model/threads";
import { WorkspaceFile } from "~/lib/model/workspace";
import { cn } from "~/lib/utils";

import { Badge } from "~/components/ui/badge";
import { Card, CardContent } from "~/components/ui/card";

import {
    Accordion,
    AccordionContent,
    AccordionItem,
    AccordionTrigger,
} from "../ui/accordion";
import { Button } from "../ui/button";

interface ThreadMetaProps {
    thread: Thread;
    agent: Agent;
    files: WorkspaceFile[];
    knowledge: KnowledgeFile[];
    className?: string;
}

export function ThreadMeta({
    thread,
    agent,
    files,
    className,
}: ThreadMetaProps) {
    return (
        <Card className={cn("h-full bg-0", className)}>
            <CardContent className="space-y-4 pt-6">
                <div className="border dark:bg-secondary/25 rounded-md p-4 overflow-hidden">
                    <table className="w-full">
                        <tbody>
                            <tr className="border-foreground/25">
                                <td className="font-medium py-2 pr-4">
                                    Created
                                </td>
                                <td className="text-right">
                                    {new Date(thread.created).toLocaleString()}
                                </td>
                            </tr>
                            {agent.name && (
                                <tr className="border-foreground/25">
                                    <td className="font-medium py-2 pr-4">
                                        Agent
                                    </td>
                                    <td className="text-right">
                                        <div className="flex items-center justify-end gap-2">
                                            <span>{agent.name}</span>
                                            <Button
                                                variant="ghost"
                                                size="icon"
                                                asChild
                                            >
                                                <Link
                                                    to={`/agent/${agent.id}?from=/thread/${thread.id}`}
                                                >
                                                    <EditIcon className="w-4 h-4" />
                                                </Link>
                                            </Button>
                                        </div>
                                    </td>
                                </tr>
                            )}
                            <tr className="border-foreground/25">
                                <td className="font-medium py-2 pr-4">State</td>
                                <td className="text-right">
                                    <Badge
                                        variant="outline"
                                        className={cn(
                                            runStateToBadgeColor(thread.state),
                                            "text-white"
                                        )}
                                    >
                                        {thread.state}
                                    </Badge>
                                </td>
                            </tr>
                            {thread.currentRunId && (
                                <tr className="border-foreground/25">
                                    <td className="font-medium py-2 pr-4">
                                        Current Run ID
                                    </td>
                                    <td className="text-right">
                                        {thread.currentRunId}
                                    </td>
                                </tr>
                            )}
                            {thread.parentThreadId && (
                                <tr className="border-foreground/25">
                                    <td className="font-medium py-2 pr-4">
                                        Parent Thread ID
                                    </td>
                                    <td className="text-right">
                                        {thread.parentThreadId}
                                    </td>
                                </tr>
                            )}
                            {thread.lastRunId && (
                                <tr className="border-foreground/25">
                                    <td className="font-medium py-2 pr-4">
                                        Last Run ID
                                    </td>
                                    <td className="text-right">
                                        {thread.lastRunId}
                                    </td>
                                </tr>
                            )}
                        </tbody>
                    </table>
                </div>

                <Accordion type="multiple" className="mx-2">
                    {files.length > 0 && (
                        <AccordionItem value="files">
                            <AccordionTrigger>
                                <span className="flex items-center text-base">
                                    <FilesIcon className="h-4 w-4 mr-2" />
                                    Files
                                </span>
                            </AccordionTrigger>
                            <AccordionContent className="mx-4">
                                <ul className="space-y-2">
                                    {files.map((file: WorkspaceFile) => (
                                        <li
                                            key={file.name}
                                            className="flex items-center"
                                        >
                                            <FileIcon className="h-4 w-4 mr-2" />
                                            <span>{file.name}</span>
                                        </li>
                                    ))}
                                </ul>
                            </AccordionContent>
                        </AccordionItem>
                    )}
                </Accordion>
            </CardContent>
        </Card>
    );
}
