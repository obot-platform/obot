import cronstrue from "cronstrue";
import { ChevronDown } from "lucide-react";
import { FC, useEffect, useState } from "react";

import { KnowledgeSourceType } from "~/lib/model/knowledge";
import { KnowledgeService } from "~/lib/service/api/knowledgeService";

import CronDialog from "~/components/knowledge/CronDialog";
import KnowledgeSourceAvatar from "~/components/knowledge/KnowledgeSourceAvatar";
import { Button } from "~/components/ui/button";
import { Dialog, DialogContent, DialogTitle } from "~/components/ui/dialog";
import {
    DropdownMenu,
    DropdownMenuCheckboxItem,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuTrigger,
} from "~/components/ui/dropdown-menu";
import { Input } from "~/components/ui/input";
import { Label } from "~/components/ui/label";
import { RadioGroup, RadioGroupItem } from "~/components/ui/radio-group";

interface AddSourceModalProps {
    agentId: string;
    sourceType: KnowledgeSourceType;
    startPolling: () => void;
    isOpen: boolean;
    onOpenChange: (open: boolean) => void;
    onSave: (knowledgeSourceId: string) => void;
}

const AddSourceModal: FC<AddSourceModalProps> = ({
    agentId,
    sourceType,
    startPolling,
    isOpen,
    onOpenChange,
    onSave,
}) => {
    const [newWebsite, setNewWebsite] = useState("");
    const [newLink, setNewLink] = useState("");
    const [autoApprove, setAutoApprove] = useState(false);
    const [syncSchedule, setSyncSchedule] = useState<string>("");
    const [isCronDialogOpen, setIsCronDialogOpen] = useState(false);
    const [cronDescription, setCronDescription] = useState("");

    useEffect(() => {
        if (syncSchedule !== "") {
            try {
                setCronDescription(cronstrue.toString(syncSchedule));
            } catch (_) {
                setCronDescription("Invalid cron expression");
            }
        }
    }, [syncSchedule]);

    const handleAddWebsite = async () => {
        if (newWebsite) {
            const trimmedWebsite = newWebsite.trim();
            const formattedWebsite =
                trimmedWebsite.startsWith("http://") ||
                trimmedWebsite.startsWith("https://")
                    ? trimmedWebsite
                    : `https://${trimmedWebsite}`;

            const res = await KnowledgeService.createKnowledgeSource(agentId, {
                websiteCrawlingConfig: {
                    urls: [formattedWebsite],
                },
                syncSchedule: syncSchedule,
                autoApprove: autoApprove,
            });
            onSave(res.id);
            startPolling();
            setNewWebsite("");
            setSyncSchedule("");
            setAutoApprove(false);
            onOpenChange(false);
        }
    };

    const handleAddOneDrive = async () => {
        const res = await KnowledgeService.createKnowledgeSource(agentId, {
            onedriveConfig: {
                sharedLinks: [newLink.trim()],
            },
            syncSchedule: syncSchedule,
            autoApprove: autoApprove,
        });
        onSave(res.id);
        setNewLink("");
        setSyncSchedule("");
        setAutoApprove(false);
        startPolling();
        onOpenChange(false);
    };

    const handleAddNotion = async () => {
        const res = await KnowledgeService.createKnowledgeSource(agentId, {
            notionConfig: {},
            syncSchedule: syncSchedule,
            autoApprove: autoApprove,
        });
        onSave(res.id);
        startPolling();
        setSyncSchedule("");
        setAutoApprove(false);
        onOpenChange(false);
    };

    const handleAdd = async () => {
        if (sourceType === KnowledgeSourceType.Website) {
            await handleAddWebsite();
        } else if (sourceType === KnowledgeSourceType.OneDrive) {
            await handleAddOneDrive();
        } else if (sourceType === KnowledgeSourceType.Notion) {
            await handleAddNotion();
        }
        startPolling();
        onOpenChange(false);
    };

    return (
        <Dialog open={isOpen} onOpenChange={onOpenChange}>
            <DialogContent aria-describedby={undefined} className="max-w-2xl">
                <DialogTitle className="flex flex-row items-center text-xl font-semibold mb-4 justify-between">
                    <div className="flex flex-row items-center">
                        <KnowledgeSourceAvatar
                            knowledgeSourceType={sourceType}
                        />
                        Add {sourceType}
                    </div>
                </DialogTitle>
                <div className="mb-4">
                    {sourceType !== KnowledgeSourceType.Notion && (
                        <div className="w-full grid grid-cols-2 items-center justify-center gap-2 mb-4">
                            <Label
                                htmlFor="site"
                                className="block text-sm font-medium text-center"
                            >
                                {sourceType === KnowledgeSourceType.Website &&
                                    "Site"}
                                {sourceType === KnowledgeSourceType.OneDrive &&
                                    "Link URL"}
                            </Label>
                            <Input
                                id="site"
                                type="text"
                                value={
                                    sourceType === KnowledgeSourceType.Website
                                        ? newWebsite
                                        : newLink
                                }
                                onChange={(e) =>
                                    sourceType === KnowledgeSourceType.Website
                                        ? setNewWebsite(e.target.value)
                                        : setNewLink(e.target.value)
                                }
                                placeholder={
                                    sourceType === KnowledgeSourceType.Website
                                        ? "Enter website URL"
                                        : "Enter OneDrive folder link"
                                }
                                className="w-[250px] dark:bg-secondary"
                            />
                        </div>
                    )}
                    <div className="w-full grid grid-cols-2 items-center justify-center gap-2 mb-4">
                        <Label
                            htmlFor="scrapeSchedule"
                            className="block text-sm font-medium text-center"
                        >
                            {sourceType === KnowledgeSourceType.Website
                                ? "Scrape Schedule"
                                : "Sync Schedule"}
                        </Label>
                        <DropdownMenu>
                            <DropdownMenuTrigger asChild>
                                <div className="w-[250px] flex flex-row items-center justify-between">
                                    <Button
                                        className="w-full mb-2 bg-secondary justify-between text-light"
                                        variant="outline"
                                    >
                                        <span className="text-gray-500">
                                            {syncSchedule === ""
                                                ? "On-Demand"
                                                : cronDescription}
                                        </span>
                                        <ChevronDown className="h-4 w-4" />
                                    </Button>
                                </div>
                            </DropdownMenuTrigger>
                            <DropdownMenuContent className="w-[250px]">
                                <DropdownMenuItem
                                    className="cursor-pointer"
                                    onClick={() => setSyncSchedule("")}
                                    defaultChecked
                                >
                                    On-Demand
                                    <DropdownMenuCheckboxItem
                                        checked={syncSchedule === ""}
                                    />
                                </DropdownMenuItem>
                                <DropdownMenuItem
                                    className="cursor-pointer"
                                    onClick={() => setSyncSchedule("0 * * * *")}
                                >
                                    Hourly
                                    <DropdownMenuCheckboxItem
                                        checked={syncSchedule === "0 * * * *"}
                                    />
                                </DropdownMenuItem>
                                <DropdownMenuItem
                                    className="cursor-pointer"
                                    onClick={() => setSyncSchedule("0 0 * * *")}
                                >
                                    Daily
                                    <DropdownMenuCheckboxItem
                                        checked={syncSchedule === "0 0 * * *"}
                                    />
                                </DropdownMenuItem>
                                <DropdownMenuItem
                                    className="cursor-pointer"
                                    onClick={() => setSyncSchedule("0 0 * * 0")}
                                >
                                    Weekly
                                    <DropdownMenuCheckboxItem
                                        checked={syncSchedule === "0 0 * * 0"}
                                    />
                                </DropdownMenuItem>
                                <DropdownMenuItem
                                    className="cursor-pointer"
                                    onClick={() => setIsCronDialogOpen(true)}
                                >
                                    <span>Custom</span>
                                    <DropdownMenuCheckboxItem
                                        checked={
                                            ![
                                                "0 * * * *",
                                                "0 0 * * *",
                                                "0 0 * * 0",
                                            ].includes(syncSchedule ?? "") &&
                                            syncSchedule !== ""
                                        }
                                    />
                                </DropdownMenuItem>
                            </DropdownMenuContent>
                        </DropdownMenu>
                    </div>
                    <div className="w-full grid grid-cols-2 items-center justify-center gap-2 mb-4">
                        <Label className="block text-sm font-medium mt-4 text-center">
                            New Files Ingestion Policy:
                        </Label>
                        <RadioGroup
                            value={autoApprove ? "auto-approve" : "default"}
                        >
                            <div className="flex flex-col gap-2 justify-center">
                                <div className="flex items-center space-x-2">
                                    <RadioGroupItem
                                        value="default"
                                        id="r1"
                                        onClick={() => setAutoApprove(false)}
                                    />
                                    <Label htmlFor="r1" className="text-sm">
                                        Manual
                                    </Label>
                                </div>
                                <div className="flex items-center space-x-2">
                                    <RadioGroupItem
                                        value="auto-approve"
                                        id="r2"
                                        onClick={() => setAutoApprove(true)}
                                    />
                                    <Label htmlFor="r2" className=" text-sm">
                                        Automatic
                                    </Label>
                                </div>
                            </div>
                        </RadioGroup>
                    </div>
                </div>
                <div className="flex justify-end gap-2">
                    <Button
                        onClick={handleAdd}
                        className="w-1/2"
                        variant="secondary"
                    >
                        OK
                    </Button>
                    <Button
                        onClick={() => onOpenChange(false)}
                        className="w-1/2"
                        variant="secondary"
                    >
                        Cancel
                    </Button>
                </div>
                <CronDialog
                    isOpen={isCronDialogOpen}
                    onOpenChange={setIsCronDialogOpen}
                    cronExpression={syncSchedule}
                    setCronExpression={setSyncSchedule}
                />
            </DialogContent>
        </Dialog>
    );
};

export default AddSourceModal;
