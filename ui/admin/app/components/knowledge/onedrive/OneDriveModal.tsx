import {
    ChevronDown,
    ChevronUp,
    FileIcon,
    FolderIcon,
    Plus,
    Trash,
} from "lucide-react";
import { FC, useEffect, useState } from "react";

import { RemoteKnowledgeSource } from "~/lib/model/knowledge";
import { KnowledgeService } from "~/lib/service/api/knowledgeService";

import RemoteKnowledgeSourceStatus from "~/components/knowledge/RemoteKnowledgeSourceStatus";
import { LoadingSpinner } from "~/components/ui/LoadingSpinner";
import { Avatar } from "~/components/ui/avatar";
import { Button } from "~/components/ui/button";
import { Dialog, DialogContent, DialogTitle } from "~/components/ui/dialog";
import { Input } from "~/components/ui/input";
import { ScrollArea } from "~/components/ui/scroll-area";
import { Table, TableBody, TableCell, TableRow } from "~/components/ui/table";

interface OnedriveModalProps {
    agentId: string;
    isOpen: boolean;
    onOpenChange: (open: boolean) => void;
    remoteKnowledgeSources: RemoteKnowledgeSource[];
    getRemoteKnowledgeSources: any;
    startPolling: () => void;
}

export const OnedriveModal: FC<OnedriveModalProps> = ({
    agentId,
    isOpen,
    onOpenChange,
    remoteKnowledgeSources,
    startPolling,
    getRemoteKnowledgeSources,
}) => {
    const [links, setLinks] = useState<string[]>([]);
    const [newLink, setNewLink] = useState("");
    const [exclude, setExclude] = useState<string[]>([]);
    const [showTable, setShowTable] = useState<{ [key: number]: boolean }>({});
    const onedriveSource = remoteKnowledgeSources.find(
        (source) => source.sourceType === "onedrive"
    );

    const isSyncing = links.length > 0 && onedriveSource?.runID;

    useEffect(() => {
        setLinks(onedriveSource?.onedriveConfig?.sharedLinks || []);
    }, [onedriveSource]);

    const handleAddLink = () => {
        if (newLink) {
            handleSave([...links, newLink], false);
            setLinks([...links, newLink]);
            setNewLink("");
        }
    };

    const handleRemoveLink = (index: number) => {
        setLinks(links.filter((_, i) => i !== index));
        handleSave(
            links.filter((_, i) => i !== index),
            false
        );
    };

    const handleSave = async (links: string[], ingest: boolean) => {
        const remoteKnowledgeSources =
            await KnowledgeService.getRemoteKnowledgeSource(agentId);
        const onedriveSource = remoteKnowledgeSources.find(
            (source) => source.sourceType === "onedrive"
        );
        if (!onedriveSource) {
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
            onedriveSource.id,
            {
                sourceType: "onedrive",
                onedriveConfig: {
                    sharedLinks: links,
                },
                exclude: exclude,
                disableIngestionAfterSync: !ingest,
            }
        );
        const intervalId = setInterval(() => {
            getRemoteKnowledgeSources.mutate();
            if (onedriveSource?.runID) {
                clearInterval(intervalId);
            }
        }, 1000);
        setTimeout(() => {
            clearInterval(intervalId);
        }, 10000);
        if (ingest) {
            await KnowledgeService.triggerKnowledgeIngestion(agentId);
            onOpenChange(false);
        }
        startPolling();
    };

    const handleTogglePageSelection = (url: string) => {
        if (exclude.includes(url)) {
            setExclude(exclude.filter((u) => u !== url));
        } else {
            setExclude([...exclude, url]);
        }
    };

    const handleClose = async (open: boolean) => {
        if (!open && onedriveSource) {
            await KnowledgeService.updateRemoteKnowledgeSource(
                agentId,
                onedriveSource.id,
                {
                    sourceType: "onedrive",
                    onedriveConfig: {
                        sharedLinks: onedriveSource.onedriveConfig?.sharedLinks,
                    },
                    exclude: onedriveSource.exclude,
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
                <DialogTitle className="flex flex-row items-center text-xl font-semibold mb-4">
                    <Avatar className="flex-row items-center w-6 h-6 mr-2">
                        <img src="/onedrive.svg" alt="OneDrive logo" />
                    </Avatar>
                    OneDrive
                </DialogTitle>
                <div className="mb-4">
                    <Input
                        type="text"
                        value={newLink}
                        onChange={(e) => setNewLink(e.target.value)}
                        placeholder="Enter OneDrive link"
                        className="w-full mb-4"
                    />
                    <Button onClick={handleAddLink} className="w-full">
                        <Plus className="mr-2 h-4 w-4" /> Add Link
                    </Button>
                </div>
                <ScrollArea className="max-h-[800px] overflow-x-auto">
                    <div className="max-h-[400px] overflow-x-auto">
                        {links.map((link, index) => (
                            <div key={index}>
                                <div
                                    key={index}
                                    className="flex flex-row items-center justify-between overflow-x-auto pr-4 h-12 cursor-pointer"
                                    onClick={() => {
                                        if (
                                            showTable[index] === undefined ||
                                            showTable[index] === false
                                        ) {
                                            setShowTable((prev) => ({
                                                ...prev,
                                                [index]: true,
                                            }));
                                        } else {
                                            setShowTable((prev) => ({
                                                ...prev,
                                                [index]: false,
                                            }));
                                        }
                                    }}
                                >
                                    <span className="flex-1 mr-2 overflow-x-auto whitespace-nowrap pr-10 scrollbar-hide flex flex-row items-center">
                                        {onedriveSource?.state?.onedriveState
                                            ?.links?.[link]?.name ? (
                                            onedriveSource?.state?.onedriveState
                                                ?.links?.[link]?.isFolder ? (
                                                <FolderIcon className="mr-2 h-4 w-4 align-middle" />
                                            ) : (
                                                <FileIcon className="mr-2 h-4 w-4" />
                                            )
                                        ) : (
                                            <Avatar className="mr-2 h-4 w-4">
                                                <img
                                                    src="/onedrive.svg"
                                                    alt="OneDrive logo"
                                                />
                                            </Avatar>
                                        )}

                                        {onedriveSource?.state?.onedriveState
                                            ?.links?.[link]?.name ? (
                                            <a
                                                href={link}
                                                target="_blank"
                                                rel="noopener noreferrer"
                                                className="underline align-middle"
                                            >
                                                {
                                                    onedriveSource?.state
                                                        ?.onedriveState
                                                        ?.links?.[link]?.name
                                                }
                                            </a>
                                        ) : (
                                            <span className="flex items-center">
                                                Processing OneDrive link...
                                                <LoadingSpinner className="ml-2 h-4 w-4" />
                                            </span>
                                        )}
                                    </span>
                                    <Button
                                        variant="ghost"
                                        onClick={(e) => {
                                            e.stopPropagation();
                                            handleRemoveLink(index);
                                        }}
                                    >
                                        <Trash className="h-4 w-4" />
                                    </Button>
                                    {onedriveSource?.state?.onedriveState
                                        ?.links?.[link]?.isFolder &&
                                        (showTable[index] ? (
                                            <ChevronUp className="h-4 w-4" />
                                        ) : (
                                            <ChevronDown className="h-4 w-4" />
                                        ))}
                                </div>
                                {showTable[index] && (
                                    <ScrollArea className="max-h-[200px] overflow-x-auto mb-2">
                                        <Table className="min-w-full divide-y divide-gray-200">
                                            <TableBody>
                                                {Object.entries(
                                                    onedriveSource?.state
                                                        ?.onedriveState
                                                        ?.files || {}
                                                )
                                                    .filter(([_, file]) =>
                                                        file.folderPath?.startsWith(
                                                            onedriveSource
                                                                ?.state
                                                                ?.onedriveState
                                                                ?.links?.[link]
                                                                ?.name!
                                                        )
                                                    )
                                                    .map(
                                                        (
                                                            [fileID, file],
                                                            index: number
                                                        ) => (
                                                            <TableRow
                                                                key={index}
                                                                className="border-t"
                                                                onClick={() =>
                                                                    handleTogglePageSelection(
                                                                        fileID
                                                                    )
                                                                }
                                                            >
                                                                <TableCell className="px-4 py-2">
                                                                    <input
                                                                        type="checkbox"
                                                                        checked={
                                                                            !exclude.includes(
                                                                                fileID
                                                                            )
                                                                        }
                                                                        onChange={() =>
                                                                            handleTogglePageSelection(
                                                                                fileID
                                                                            )
                                                                        }
                                                                        onClick={(
                                                                            e
                                                                        ) =>
                                                                            e.stopPropagation()
                                                                        }
                                                                    />
                                                                </TableCell>
                                                                <TableCell className="px-4 py-2">
                                                                    <a
                                                                        href={
                                                                            file.url
                                                                        }
                                                                        target="_blank"
                                                                        rel="noopener noreferrer"
                                                                        className="underline"
                                                                        onClick={(
                                                                            e
                                                                        ) =>
                                                                            e.stopPropagation()
                                                                        }
                                                                    >
                                                                        {
                                                                            file.fileName
                                                                        }
                                                                    </a>
                                                                    {file.folderPath && (
                                                                        <>
                                                                            <br />
                                                                            <span className="text-gray-400 text-xs">
                                                                                {
                                                                                    file.folderPath
                                                                                }
                                                                            </span>
                                                                        </>
                                                                    )}
                                                                </TableCell>
                                                            </TableRow>
                                                        )
                                                    )}
                                            </TableBody>
                                        </Table>
                                    </ScrollArea>
                                )}
                            </div>
                        ))}
                    </div>
                </ScrollArea>
                {isSyncing && (
                    <RemoteKnowledgeSourceStatus source={onedriveSource!} />
                )}

                <div className="mt-4 flex justify-end">
                    <Button onClick={() => handleSave(links, true)}>
                        Sync
                    </Button>
                </div>
            </DialogContent>
        </Dialog>
    );
};
