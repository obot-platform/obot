import { ChevronDown, ChevronUp, Globe, Plus, Trash, X } from "lucide-react";
import { FC, useEffect, useState } from "react";

import { RemoteKnowledgeSource } from "~/lib/model/knowledge";
import { KnowledgeService } from "~/lib/service/api/knowledgeService";

import { Avatar } from "~/components/ui/avatar";
import { Button } from "~/components/ui/button";
import { Dialog, DialogContent, DialogTitle } from "~/components/ui/dialog";
import { Input } from "~/components/ui/input";
import { ScrollArea } from "~/components/ui/scroll-area";
import { Table, TableBody, TableCell, TableRow } from "~/components/ui/table";

import RemoteKnowledgeSourceStatus from "../RemoteKnowledgeSourceStatus";

interface WebsiteModalProps {
    agentId: string;
    isOpen: boolean;
    onOpenChange: (open: boolean) => void;
    remoteKnowledgeSources: RemoteKnowledgeSource[];
    getRemoteKnowledgeSources: any;
    startPolling: () => void;
}

export const WebsiteModal: FC<WebsiteModalProps> = ({
    agentId,
    isOpen,
    onOpenChange,
    remoteKnowledgeSources,
    getRemoteKnowledgeSources,
    startPolling,
}) => {
    const [websites, setWebsites] = useState<string[]>([]);
    const [newWebsite, setNewWebsite] = useState("");
    const [exclude, setExclude] = useState<string[]>([]);
    const [showTable, setShowTable] = useState<{ [key: number]: boolean }>({});

    const websiteSource = remoteKnowledgeSources.find(
        (source) => source.sourceType === "website"
    );

    const isSyncing = websites.length > 0 && websiteSource?.runID;

    useEffect(() => {
        setExclude(websiteSource?.exclude || []);
        setWebsites(websiteSource?.websiteCrawlingConfig?.urls || []);
    }, [websiteSource]);

    const handleSave = async (websites: string[], ingest: boolean = false) => {
        const remoteKnowledgeSources =
            await KnowledgeService.getRemoteKnowledgeSource(agentId);
        let websiteSource = remoteKnowledgeSources.find(
            (source) => source.sourceType === "website"
        );
        if (!websiteSource) {
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
            websiteSource.id,
            {
                sourceType: "website",
                websiteCrawlingConfig: {
                    urls: websites,
                },
                exclude: exclude,
                disableIngestionAfterSync: !ingest,
            }
        );
        const intervalId = setInterval(() => {
            getRemoteKnowledgeSources.mutate();
            if (websiteSource?.runID) {
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

    const handleAddWebsite = async () => {
        if (newWebsite) {
            const formattedWebsite =
                newWebsite.startsWith("http://") ||
                newWebsite.startsWith("https://")
                    ? newWebsite
                    : `https://${newWebsite}`;
            setWebsites((prevWebsites) => {
                const updatedWebsites = [...prevWebsites, formattedWebsite];
                handleSave(updatedWebsites);
                return updatedWebsites;
            });
            setNewWebsite("");
        }
    };

    const handleRemoveWebsite = async (index: number) => {
        setWebsites(websites.filter((_, i) => i !== index));
        await handleSave(websites.filter((_, i) => i !== index));
    };

    useEffect(() => {
        const fetchWebsites = async () => {
            const remoteKnowledgeSources =
                await KnowledgeService.getRemoteKnowledgeSource(agentId);
            const websiteSource = remoteKnowledgeSources.find(
                (source) => source.sourceType === "website"
            );
            setWebsites(websiteSource?.websiteCrawlingConfig?.urls || []);
        };

        fetchWebsites();
    }, [agentId]);

    const handleTogglePageSelection = (url: string) => {
        setExclude((prev) =>
            prev.includes(url)
                ? prev.filter((item) => item !== url)
                : [...prev, url]
        );
    };

    const handleClose = async (open: boolean) => {
        if (!open && websiteSource) {
            await KnowledgeService.updateRemoteKnowledgeSource(
                agentId,
                websiteSource.id,
                {
                    sourceType: "website",
                    websiteCrawlingConfig: {
                        urls: websiteSource.websiteCrawlingConfig?.urls,
                    },
                    exclude: websiteSource.exclude,
                    disableIngestionAfterSync: false,
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
                className="data-[state=open]:animate-contentShow fixed top-[50%] left-[50%] max-h-[85vh] w-[90vw] max-w-[900px] translate-x-[-50%] translate-y-[-50%] rounded-[6px] bg-white dark:bg-secondary p-[25px] shadow-[hsl(206_22%_7%_/_35%)_0px_10px_38px_-10px,_hsl(206_22%_7%_/_20%)_0px_10px_20px_-15px] focus:outline-none"
            >
                <DialogTitle className="flex flex-row items-center text-xl font-semibold mb-4">
                    <Avatar className="flex-row items-center w-6 h-6 mr-2">
                        <Globe className="w-4 h-4" />
                    </Avatar>
                    Website
                </DialogTitle>
                <div className="mb-4">
                    <Input
                        type="text"
                        value={newWebsite}
                        onChange={(e) => setNewWebsite(e.target.value)}
                        placeholder="Enter website URL"
                        className="w-full mb-2 dark:bg-secondary"
                    />
                    <Button onClick={handleAddWebsite} className="w-full">
                        <Plus className="mr-2 h-4 w-4" /> Add URL
                    </Button>
                </div>
                <ScrollArea className="max-h-[800px] overflow-x-auto">
                    <div className="max-h-[400px] overflow-x-auto">
                        {websites.map((website, index) => (
                            <ScrollArea className="max-h-[400px] overflow-x-auto">
                                <div
                                    key={index}
                                    className="flex items-center justify-between mb-2 overflow-x-auto"
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
                                    <span className="flex-1 mr-2 overflow-x-auto whitespace-nowrap dark:text-white">
                                        <div className="flex items-center flex-r">
                                            <Globe className="mr-2 h-4 w-4" />
                                            <a
                                                href={website}
                                                target="_blank"
                                                rel="noopener noreferrer"
                                                className="underline"
                                            >
                                                {website}
                                            </a>
                                        </div>
                                    </span>
                                    <Button
                                        variant="ghost"
                                        onClick={() =>
                                            handleRemoveWebsite(index)
                                        }
                                    >
                                        <Trash className="h-4 w-4 dark:text-white" />
                                    </Button>
                                    {showTable[index] ? (
                                        <ChevronUp className="h-4 w-4" />
                                    ) : (
                                        <ChevronDown className="h-4 w-4" />
                                    )}
                                </div>
                                {showTable[index] && (
                                    <Table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
                                        <TableBody>
                                            {Object.entries(
                                                websiteSource?.state
                                                    ?.websiteCrawlingState
                                                    ?.pages || {}
                                            )
                                                .filter(
                                                    ([url, details]) =>
                                                        details.parentUrl ===
                                                        website
                                                )
                                                .map(
                                                    (
                                                        [url, details],
                                                        index: number
                                                    ) => (
                                                        <TableRow
                                                            key={index}
                                                            className="border-t dark:border-gray-600"
                                                            onClick={() =>
                                                                handleTogglePageSelection(
                                                                    url
                                                                )
                                                            }
                                                        >
                                                            <TableCell className="px-4 py-2">
                                                                <input
                                                                    type="checkbox"
                                                                    checked={
                                                                        !exclude.includes(
                                                                            url
                                                                        )
                                                                    }
                                                                    onChange={() =>
                                                                        handleTogglePageSelection(
                                                                            url
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
                                                                    href={url}
                                                                    target="_blank"
                                                                    rel="noopener noreferrer"
                                                                    className="underline dark:text-blue-400"
                                                                    onClick={(
                                                                        e
                                                                    ) =>
                                                                        e.stopPropagation()
                                                                    }
                                                                >
                                                                    {url}
                                                                </a>
                                                            </TableCell>
                                                        </TableRow>
                                                    )
                                                )}
                                        </TableBody>
                                    </Table>
                                )}
                            </ScrollArea>
                        ))}
                    </div>
                </ScrollArea>

                {isSyncing && (
                    <RemoteKnowledgeSourceStatus source={websiteSource!} />
                )}
                <div className="mt-4 flex justify-end">
                    <Button onClick={() => handleSave(websites, true)}>
                        Sync
                    </Button>
                </div>
            </DialogContent>
        </Dialog>
    );
};
