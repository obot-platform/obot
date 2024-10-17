import { FC, useEffect, useState } from "react";

import { RemoteKnowledgeSource } from "~/lib/model/knowledge";
import { KnowledgeService } from "~/lib/service/api/knowledgeService";

import RemoteKnowledgeSourceStatus from "~/components/knowledge/RemoteKnowledgeSourceStatus";
import { Avatar } from "~/components/ui/avatar";
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
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "~/components/ui/table";

type NotionModalProps = {
    agentId: string;
    isOpen: boolean;
    onOpenChange: (open: boolean) => void;
    remoteKnowledgeSources: RemoteKnowledgeSource[];
    startPolling: () => void;
};

export const NotionModal: FC<NotionModalProps> = ({
    agentId,
    isOpen,
    onOpenChange,
    remoteKnowledgeSources,
    startPolling,
}) => {
    const [exclude, setExclude] = useState<string[]>([]);

    const notionSource = remoteKnowledgeSources.find(
        (source) => source.sourceType === "notion"
    );

    useEffect(() => {
        setExclude(notionSource?.exclude || []);
    }, [notionSource]);

    const handleSave = async () => {
        if (!notionSource) {
            return;
        }
        const knowledge = await KnowledgeService.getKnowledgeForAgent(agentId);
        for (const file of knowledge) {
            if (file.uploadID && exclude.includes(file.uploadID)) {
                await KnowledgeService.deleteKnowledgeFromAgent(
                    agentId,
                    file.fileName
                );
            }
        }

        await KnowledgeService.updateRemoteKnowledgeSource(
            agentId,
            notionSource.id,
            {
                sourceType: "notion",
                exclude: exclude,
                disableIngestionAfterSync: false,
            }
        );
        await KnowledgeService.triggerKnowledgeIngestion(agentId);
        onOpenChange(false);
        startPolling();
    };

    const handleClose = async (open: boolean) => {
        if (!open && notionSource) {
            await KnowledgeService.updateRemoteKnowledgeSource(
                agentId,
                notionSource.id,
                {
                    sourceType: "notion",
                }
            );
            await KnowledgeService.triggerKnowledgeIngestion(agentId);
        }
        onOpenChange(open);
    };

    return (
        <Dialog open={isOpen} onOpenChange={handleClose}>
            <DialogContent
                aria-describedby={undefined}
                className="bd-secondary data-[state=open]:animate-contentShow fixed top-[50%] left-[50%] max-h-[85vh] w-[90vw] max-w-[900px] translate-x-[-50%] translate-y-[-50%] rounded-[6px] bg-white dark:bg-secondary p-[25px] shadow-[hsl(206_22%_7%_/_35%)_0px_10px_38px_-10px,_hsl(206_22%_7%_/_20%)_0px_10px_20px_-15px] focus:outline-none"
            >
                <DialogHeader>
                    <DialogTitle className="flex flex-row items-center text-xl font-semibold mb-4">
                        <Avatar className="flex-row items-center w-6 h-6 mr-2">
                            <img src="/notion.svg" alt="Notion logo" />
                        </Avatar>
                        Notion
                    </DialogTitle>
                </DialogHeader>
                <ScrollArea>
                    {notionSource?.state?.notionState?.pages && (
                        <Table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
                            <TableHeader className="bg-gray-50 dark:bg-secondary">
                                <TableRow>
                                    <TableHead className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                                        Pages
                                    </TableHead>
                                </TableRow>
                            </TableHeader>
                            <TableBody className="bg-white dark:bg-secondary divide-y divide-gray-200 dark:divide-gray-700">
                                {Object.entries(
                                    notionSource?.state?.notionState?.pages ||
                                        {}
                                )
                                    .sort(([, pageA], [, pageB]) =>
                                        (pageA?.folderPath || "").localeCompare(
                                            pageB?.folderPath || ""
                                        )
                                    )
                                    .map(([id, page]) => (
                                        <TableRow
                                            key={id}
                                            className="hover:bg-gray-100 dark:hover:bg-gray-800"
                                        >
                                            <TableCell
                                                className="px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900 dark:text-gray-100 flex items-center cursor-pointer"
                                                onClick={() =>
                                                    setExclude((prevExclude) =>
                                                        prevExclude.includes(id)
                                                            ? prevExclude.filter(
                                                                  (pageId) =>
                                                                      pageId !==
                                                                      id
                                                              )
                                                            : [
                                                                  ...prevExclude,
                                                                  id,
                                                              ]
                                                    )
                                                }
                                            >
                                                <Input
                                                    type="checkbox"
                                                    checked={
                                                        !exclude.includes(id)
                                                    }
                                                    onChange={() =>
                                                        setExclude(
                                                            (prevExclude) =>
                                                                prevExclude.includes(
                                                                    id
                                                                )
                                                                    ? prevExclude.filter(
                                                                          (
                                                                              pageId
                                                                          ) =>
                                                                              pageId !==
                                                                              id
                                                                      )
                                                                    : [
                                                                          ...prevExclude,
                                                                          id,
                                                                      ]
                                                        )
                                                    }
                                                    className="mr-3 h-4 w-4"
                                                    onClick={(e) =>
                                                        e.stopPropagation()
                                                    }
                                                />
                                                <div>
                                                    <a
                                                        href={page.url}
                                                        target="_blank"
                                                        rel="noopener noreferrer"
                                                        className="text-black-600 dark:text-gray-100 hover:underline"
                                                        onClick={(e) =>
                                                            e.stopPropagation()
                                                        }
                                                    >
                                                        {page.title}
                                                    </a>
                                                    <div className="text-gray-400 dark:text-gray-500 text-xs">
                                                        {page.folderPath}
                                                    </div>
                                                </div>
                                            </TableCell>
                                        </TableRow>
                                    ))}
                            </TableBody>
                        </Table>
                    )}
                </ScrollArea>
                {notionSource?.runID && (
                    <RemoteKnowledgeSourceStatus source={notionSource!} />
                )}
                <DialogFooter>
                    <Button onClick={handleSave}>Sync</Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
};
