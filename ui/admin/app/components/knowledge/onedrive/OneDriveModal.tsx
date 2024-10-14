import { Plus, X } from "lucide-react";
import { FC, useEffect, useState } from "react";

import { RemoteKnowledgeSource } from "~/lib/model/knowledge";
import { KnowledgeService } from "~/lib/service/api/knowledgeService";

import RemoteKnowledgeSourceStatus from "~/components/knowledge/RemoteKnowledgeSourceStatus";
import { Button } from "~/components/ui/button";
import { Dialog, DialogContent, DialogTitle } from "~/components/ui/dialog";
import { Input } from "~/components/ui/input";
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "~/components/ui/table";

interface OnedriveModalProps {
    agentId: string;
    isOpen: boolean;
    onOpenChange: (open: boolean) => void;
    startPolling: () => void;
    remoteKnowledgeSources: RemoteKnowledgeSource[];
}

export const OnedriveModal: FC<OnedriveModalProps> = ({
    agentId,
    isOpen,
    onOpenChange,
    startPolling,
    remoteKnowledgeSources,
}) => {
    const [links, setLinks] = useState<string[]>([]);
    const [newLink, setNewLink] = useState("");
    const [exclude, setExclude] = useState<string[]>([]);
    const onedriveSource = remoteKnowledgeSources.find(
        (source) => source.sourceType === "onedrive"
    );

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
            await KnowledgeService.createRemoteKnowledgeSource(agentId, {
                sourceType: "onedrive",
                onedriveConfig: {
                    sharedLinks: links,
                },
                disableIngestionAfterSync: !ingest,
            });
        } else {
            const knowledge =
                await KnowledgeService.getKnowledgeForAgent(agentId);
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
        }
        startPolling();
        if (ingest) {
            await KnowledgeService.triggerKnowledgeIngestion(agentId);
            onOpenChange(false);
        }
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
                <DialogTitle className="text-xl font-semibold mb-4">
                    Add OneDrive Links
                </DialogTitle>
                <div className="mb-4">
                    <Input
                        type="text"
                        value={newLink}
                        onChange={(e) => setNewLink(e.target.value)}
                        placeholder="Enter OneDrive link"
                        className="w-full mb-2"
                    />
                    <Button onClick={handleAddLink} className="w-full">
                        <Plus className="mr-2 h-4 w-4" /> Add Link
                    </Button>
                </div>
                <div className="max-h-[200px] overflow-x-auto">
                    {links.map((link, index) => (
                        <div
                            key={index}
                            className="flex items-center justify-between mb-2 overflow-x-auto"
                        >
                            <span className="flex-1 mr-2 overflow-x-auto whitespace-nowrap">
                                {link}
                            </span>
                            <Button
                                variant="ghost"
                                onClick={() => handleRemoveLink(index)}
                            >
                                <X className="h-4 w-4" />
                            </Button>
                        </div>
                    ))}
                </div>
                <div className="max-h-[200px] overflow-x-auto mb-4">
                    {links.length > 0 &&
                    Object.keys(
                        onedriveSource?.state?.onedriveState?.files || {}
                    ).length > 0 &&
                    !onedriveSource?.runID ? (
                        <>
                            <Table className="min-w-full divide-y divide-gray-200">
                                <TableHeader className="bg-gray-50 dark:bg-secondary">
                                    <TableRow>
                                        <TableHead className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                                            Files
                                        </TableHead>
                                    </TableRow>
                                </TableHeader>
                                <TableBody>
                                    {Object.entries(
                                        onedriveSource?.state?.onedriveState
                                            ?.files || {}
                                    ).map(([fileID, file], index: number) => (
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
                                                    onClick={(e) =>
                                                        e.stopPropagation()
                                                    }
                                                />
                                            </TableCell>
                                            <TableCell className="px-4 py-2">
                                                <a
                                                    href={file.url}
                                                    target="_blank"
                                                    rel="noopener noreferrer"
                                                    className="underline"
                                                    onClick={(e) =>
                                                        e.stopPropagation()
                                                    }
                                                >
                                                    {file.fileName}
                                                </a>
                                                {file.folderPath && (
                                                    <>
                                                        <br />
                                                        <span className="text-gray-400 text-xs">
                                                            {file.folderPath}
                                                        </span>
                                                    </>
                                                )}
                                            </TableCell>
                                        </TableRow>
                                    ))}
                                </TableBody>
                            </Table>
                        </>
                    ) : (
                        <RemoteKnowledgeSourceStatus source={onedriveSource!} />
                    )}
                </div>
                <div className="mt-4 flex justify-end">
                    <Button onClick={() => handleSave(links, true)}>
                        Save
                    </Button>
                </div>
            </DialogContent>
        </Dialog>
    );
};
