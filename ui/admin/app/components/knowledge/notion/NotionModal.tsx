import { FC, useEffect, useState } from "react";

import { RemoteKnowledgeSource } from "~/lib/model/knowledge";
import { KnowledgeService } from "~/lib/service/api/knowledgeService";

import RemoteKnowledgeSourceStatus from "~/components/knowledge/RemoteKnowledgeSourceStatus";
import { Button } from "~/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogFooter,
    DialogHeader,
    DialogTitle,
} from "~/components/ui/dialog";
import { Input } from "~/components/ui/input";
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
    startPolling: () => void;
    remoteKnowledgeSources: RemoteKnowledgeSource[];
};

export const NotionModal: FC<NotionModalProps> = ({
    agentId,
    isOpen,
    onOpenChange,
    startPolling,
    remoteKnowledgeSources,
}) => {
    const [selectedPages, setSelectedPages] = useState<string[]>([]);

    const notionSource = remoteKnowledgeSources.find(
        (source) => source.sourceType === "notion"
    );

    useEffect(() => {
        setSelectedPages(notionSource?.notionConfig?.pages || []);
    }, [notionSource]);

    const handleSave = async () => {
        if (!notionSource) {
            return;
        }
        await KnowledgeService.updateRemoteKnowledgeSource(
            agentId,
            notionSource.id,
            {
                sourceType: "notion",
                notionConfig: {
                    pages: selectedPages,
                },
                exclude: notionSource.exclude,
            }
        );
        startPolling();
        onOpenChange(false);
    };
    return (
        <Dialog open={isOpen} onOpenChange={onOpenChange}>
            <DialogContent
                aria-describedby={undefined}
                className="bd-secondary data-[state=open]:animate-contentShow fixed top-[50%] left-[50%] max-h-[85vh] w-[90vw] max-w-[900px] translate-x-[-50%] translate-y-[-50%] rounded-[6px] bg-white dark:bg-secondary p-[25px] shadow-[hsl(206_22%_7%_/_35%)_0px_10px_38px_-10px,_hsl(206_22%_7%_/_20%)_0px_10px_20px_-15px] focus:outline-none"
            >
                <DialogHeader>
                    <DialogTitle>Select Notion Pages</DialogTitle>
                </DialogHeader>
                <div className="overflow-auto max-h-[400px]">
                    {notionSource?.state?.notionState?.pages &&
                    !notionSource.runID ? (
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
                                                    setSelectedPages(
                                                        (prevSelectedPages) =>
                                                            prevSelectedPages.includes(
                                                                id
                                                            )
                                                                ? prevSelectedPages.filter(
                                                                      (
                                                                          pageId
                                                                      ) =>
                                                                          pageId !==
                                                                          id
                                                                  )
                                                                : [
                                                                      ...prevSelectedPages,
                                                                      id,
                                                                  ]
                                                    )
                                                }
                                            >
                                                <Input
                                                    type="checkbox"
                                                    checked={selectedPages.includes(
                                                        id
                                                    )}
                                                    onChange={() =>
                                                        setSelectedPages(
                                                            (
                                                                prevSelectedPages
                                                            ) =>
                                                                prevSelectedPages.includes(
                                                                    id
                                                                )
                                                                    ? prevSelectedPages.filter(
                                                                          (
                                                                              pageId
                                                                          ) =>
                                                                              pageId !==
                                                                              id
                                                                      )
                                                                    : [
                                                                          ...prevSelectedPages,
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
                    ) : (
                        <RemoteKnowledgeSourceStatus source={notionSource!} />
                    )}
                </div>
                <DialogFooter>
                    <Button onClick={handleSave}>Save</Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
};
