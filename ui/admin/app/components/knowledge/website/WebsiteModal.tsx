import { Plus, X } from "lucide-react";
import { FC, useEffect, useState } from "react";

import { RemoteKnowledgeSource } from "~/lib/model/knowledge";
import { KnowledgeService } from "~/lib/service/api/knowledgeService";

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

import RemoteKnowledgeSourceStatus from "../RemoteKnowledgeSourceStatus";

interface WebsiteModalProps {
    agentId: string;
    isOpen: boolean;
    onOpenChange: (open: boolean) => void;
    startPolling: () => void;
    remoteKnowledgeSources: RemoteKnowledgeSource[];
}

export const WebsiteModal: FC<WebsiteModalProps> = ({
    agentId,
    isOpen,
    onOpenChange,
    startPolling,
    remoteKnowledgeSources,
}) => {
    const [websites, setWebsites] = useState<string[]>([]);
    const [newWebsite, setNewWebsite] = useState("");
    const [exclude, setExclude] = useState<string[]>([]);

    const websiteSource = remoteKnowledgeSources.find(
        (source) => source.sourceType === "website"
    );

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
            websiteSource = await KnowledgeService.createRemoteKnowledgeSource(
                agentId,
                {
                    sourceType: "website",
                    websiteCrawlingConfig: {
                        urls: websites,
                    },
                    disableIngestionAfterSync: !ingest,
                }
            );
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
        }
        startPolling();
        if (ingest) {
            await KnowledgeService.triggerKnowledgeIngestion(agentId);
            onOpenChange(false);
        }
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
                <DialogTitle className="dark:bg-secondary text-xl font-semibold mb-4">
                    Add Website URLs
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
                <div className="max-h-[200px] overflow-x-auto">
                    {websites.map((website, index) => (
                        <div
                            key={index}
                            className="flex items-center justify-between mb-2 overflow-x-auto"
                        >
                            <span className="flex-1 mr-2 overflow-x-auto whitespace-nowrap dark:text-white">
                                {website}
                            </span>
                            <Button
                                variant="ghost"
                                onClick={() => handleRemoveWebsite(index)}
                            >
                                <X className="h-4 w-4 dark:text-white" />
                            </Button>
                        </div>
                    ))}
                </div>
                <div className="max-h-[200px] overflow-x-auto mb-4">
                    {websites.length > 0 &&
                    Object.keys(
                        websiteSource?.state?.websiteCrawlingState?.pages || {}
                    ).length > 0 &&
                    !websiteSource?.runID ? (
                        <>
                            <Table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
                                <TableHeader className="bg-gray-50 dark:bg-secondary">
                                    <TableRow>
                                        <TableHead className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-300 uppercase tracking-wider">
                                            Pages
                                        </TableHead>
                                    </TableRow>
                                </TableHeader>
                                <TableBody>
                                    {Object.keys(
                                        websiteSource?.state
                                            ?.websiteCrawlingState?.pages || {}
                                    ).map((url, index: number) => (
                                        <TableRow
                                            key={index}
                                            className="border-t dark:border-gray-600"
                                            onClick={() =>
                                                handleTogglePageSelection(url)
                                            }
                                        >
                                            <TableCell className="px-4 py-2">
                                                <input
                                                    type="checkbox"
                                                    checked={
                                                        !exclude.includes(url)
                                                    }
                                                    onChange={() =>
                                                        handleTogglePageSelection(
                                                            url
                                                        )
                                                    }
                                                    onClick={(e) =>
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
                                                    onClick={(e) =>
                                                        e.stopPropagation()
                                                    }
                                                >
                                                    {url}
                                                </a>
                                            </TableCell>
                                        </TableRow>
                                    ))}
                                </TableBody>
                            </Table>
                        </>
                    ) : (
                        <RemoteKnowledgeSourceStatus source={websiteSource!} />
                    )}
                </div>
                <div className="mt-4 flex justify-end">
                    <Button onClick={() => handleSave(websites, true)}>
                        Save
                    </Button>
                </div>
            </DialogContent>
        </Dialog>
    );
};
