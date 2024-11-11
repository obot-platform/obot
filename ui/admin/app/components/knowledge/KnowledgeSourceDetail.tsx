import cronstrue from "cronstrue";
import {
    ArrowDownUp,
    ArrowUpDown,
    CheckIcon,
    CircleX,
    EditIcon,
    Eye,
    FileClock,
    MinusIcon,
    Plus,
    RefreshCcw,
    ShieldAlert,
    Trash,
} from "lucide-react";
import { FC, useEffect, useMemo, useRef, useState } from "react";
import useSWR, { SWRResponse } from "swr";

import {
    KnowledgeFile,
    KnowledgeFileState,
    KnowledgeSource,
    KnowledgeSourceStatus,
    KnowledgeSourceType,
    getKnowledgeFileDisplayName,
    getKnowledgeSourceDisplayName,
    getKnowledgeSourceType,
} from "~/lib/model/knowledge";
import { KnowledgeService } from "~/lib/service/api/knowledgeService";

import { TypographyP } from "~/components/Typography";
import CronDialog from "~/components/knowledge/CronDialog";
import ErrorDialog from "~/components/knowledge/ErrorDialog";
import KnowledgeSourceAvatar from "~/components/knowledge/KnowledgeSourceAvatar";
import OauthSignDialog from "~/components/knowledge/OAuthSignDialog";
import { LoadingSpinner } from "~/components/ui/LoadingSpinner";
import {
    AlertDialog,
    AlertDialogAction,
    AlertDialogCancel,
    AlertDialogContent,
    AlertDialogDescription,
    AlertDialogFooter,
    AlertDialogHeader,
    AlertDialogTitle,
    AlertDialogTrigger,
} from "~/components/ui/alert-dialog";
import { Button } from "~/components/ui/button";
import { Dialog, DialogContent, DialogTitle } from "~/components/ui/dialog";
import {
    DropdownMenu,
    DropdownMenuCheckboxItem,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuTrigger,
} from "~/components/ui/dropdown-menu";
import { Label } from "~/components/ui/label";
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "~/components/ui/table";
import {
    Tooltip,
    TooltipContent,
    TooltipTrigger,
} from "~/components/ui/tooltip";

interface KnowledgeSourceDetailProps {
    agentId: string;
    knowledgeSource: KnowledgeSource;
    onSyncNow: () => void;
    onDelete: () => void;
    isOpen: boolean;
    onOpenChange: (open: boolean) => void;
    onSave: (knowledgeSource: KnowledgeSource) => void;
}

const KnowledgeSourceDetail: FC<KnowledgeSourceDetailProps> = ({
    agentId,
    knowledgeSource,
    isOpen,
    onOpenChange,
    onSyncNow,
    onDelete,
    onSave,
}) => {
    const [blockPollingFiles, setBlockPollingFiles] = useState(true);
    const [syncSchedule, setSyncSchedule] = useState(
        knowledgeSource.syncSchedule
    );
    const [autoApprove, setAutoApprove] = useState(knowledgeSource.autoApprove);
    const [isCronDialogOpen, setIsCronDialogOpen] = useState(false);
    const [cronDescription, setCronDescription] = useState("");

    const [errorDialogError, setErrorDialogError] = useState("");
    const [sortingOrder, setSortingOrder] = useState<"asc" | "desc">("asc");
    const [sortingColumn, setSortingColumn] = useState<"fileName" | "state">(
        "fileName"
    );
    const sourceType = getKnowledgeSourceType(knowledgeSource);

    const tableContainerRef = useRef<HTMLDivElement>(null);
    const scrollPosition = useRef(0);

    useEffect(() => {
        setSyncSchedule(knowledgeSource.syncSchedule);
        setAutoApprove(knowledgeSource.autoApprove);
    }, [knowledgeSource]);

    useEffect(() => {
        if (!syncSchedule) {
            setCronDescription("");
            return;
        }
        try {
            setCronDescription(cronstrue.toString(syncSchedule));
        } catch (_) {
            setCronDescription("Invalid cron expression");
        }
    }, [syncSchedule]);

    const getFiles: SWRResponse<KnowledgeFile[], Error> = useSWR(
        KnowledgeService.getFilesForKnowledgeSource.key(
            agentId,
            knowledgeSource.id
        ),
        ({ agentId, sourceId }) =>
            KnowledgeService.getFilesForKnowledgeSource(agentId, sourceId).then(
                (files) =>
                    files.sort((a, b) => a.fileName.localeCompare(b.fileName))
            ),
        {
            revalidateOnFocus: false,
            refreshInterval: blockPollingFiles ? undefined : 1000,
        }
    );

    const files = useMemo(() => {
        if (!getFiles.data) return [];
        if (!sortingOrder) return getFiles.data;

        return [...getFiles.data].sort((a, b) => {
            const stateOrder = {
                [KnowledgeFileState.Ingesting]: 1,
                [KnowledgeFileState.Ingested]: 2,
                [KnowledgeFileState.Pending]: 3,
                [KnowledgeFileState.Error]: 4,
                [KnowledgeFileState.Unsupported]: 5,
                [KnowledgeFileState.Unapproved]: 6,
                [KnowledgeFileState.PendingApproval]: 7,
            };
            const { displayName: aDisplayName } = getKnowledgeFileDisplayName(
                a,
                knowledgeSource
            );
            const { displayName: bDisplayName } = getKnowledgeFileDisplayName(
                b,
                knowledgeSource
            );

            if (sortingColumn === "state") {
                const stateA = stateOrder[a.state];
                const stateB = stateOrder[b.state];

                if (stateA !== stateB) {
                    return sortingOrder === "asc"
                        ? stateA - stateB
                        : stateB - stateA;
                }
                return sortingOrder === "asc"
                    ? (aDisplayName?.localeCompare(bDisplayName ?? "") ?? 0)
                    : ((bDisplayName ?? "").localeCompare(aDisplayName ?? "") ??
                          0);
            } else {
                return sortingOrder === "asc"
                    ? (aDisplayName?.localeCompare(bDisplayName ?? "") ?? 0)
                    : ((bDisplayName ?? "").localeCompare(aDisplayName ?? "") ??
                          0);
            }
        });
    }, [getFiles.data, sortingOrder, sortingColumn, knowledgeSource]);

    useEffect(() => {
        if (files.length === 0) {
            setBlockPollingFiles(true);
            return;
        }

        if (
            files
                .filter(
                    (file) =>
                        file.state !== KnowledgeFileState.PendingApproval &&
                        file.state !== KnowledgeFileState.Unapproved
                )
                .every(
                    (file) =>
                        file.state === KnowledgeFileState.Ingested ||
                        file.state === KnowledgeFileState.Error
                )
        ) {
            setBlockPollingFiles(true);
        } else {
            setBlockPollingFiles(false);
        }
    }, [files]);

    useEffect(() => {
        const container = tableContainerRef.current;
        if (container) {
            container.scrollTop = scrollPosition.current;
        }
    }, [files]);

    const handleScroll = () => {
        scrollPosition.current = tableContainerRef?.current?.scrollTop ?? 0;
    };

    useEffect(() => {
        if (
            knowledgeSource.state === KnowledgeSourceStatus.Syncing ||
            knowledgeSource.state === KnowledgeSourceStatus.Pending
        ) {
            setBlockPollingFiles(false);
        }

        if (knowledgeSource.state === KnowledgeSourceStatus.Synced) {
            getFiles.mutate();
        }
    }, [knowledgeSource, getFiles]);

    const onSourceUpdate = async (
        syncSchedule: string,
        autoApprove: boolean
    ) => {
        const updatedSource = await KnowledgeService.updateKnowledgeSource(
            agentId,
            knowledgeSource.id,
            {
                ...knowledgeSource,
                syncSchedule: syncSchedule,
                autoApprove: autoApprove,
            }
        );
        onSave(updatedSource);
    };

    const onApproveFile = async (file: KnowledgeFile, approved: boolean) => {
        const updatedFile = await KnowledgeService.approveFile(
            agentId,
            file.id,
            approved
        );
        getFiles.mutate((files) =>
            files?.map((f) => (f.id === file.id ? updatedFile : f))
        );
    };

    const onApproveAllFiles = async (approved: boolean) => {
        for (const file of files) {
            await onApproveFile(file, approved);
        }
    };

    const onReingestFile = async (file: KnowledgeFile) => {
        const updatedFile = await KnowledgeService.reingestFile(
            agentId,
            file.id,
            knowledgeSource.id
        );
        getFiles.mutate((files) =>
            files?.map((f) => (f.id === file.id ? updatedFile : f))
        );
    };

    const renderFileElement = (
        file: KnowledgeFile,
        source: KnowledgeSource
    ) => {
        const { displayName, subTitle } = getKnowledgeFileDisplayName(
            file,
            source
        );

        return (
            <div className="flex flex-col overflow-hidden flex-auto">
                <a
                    href={file.url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="flex flex-col overflow-hidden flex-auto hover:underline"
                    onClick={(e) => {
                        e.stopPropagation();
                    }}
                >
                    <TypographyP className="w-full overflow-hidden text-ellipsis">
                        {displayName}
                    </TypographyP>
                </a>
                <TypographyP className="text-gray-400 text-xs">
                    {subTitle}
                </TypographyP>
            </div>
        );

        return null;
    };

    return (
        <Dialog open={isOpen} onOpenChange={onOpenChange}>
            <DialogContent
                className=" h-[80vh] w-[80vw] max-w-none max-h-none flex flex-col overflow-y-auto"
                onScroll={handleScroll}
                ref={tableContainerRef}
            >
                <DialogTitle className="flex justify-between items-center">
                    <div className="flex flex-row items-center">
                        <KnowledgeSourceAvatar
                            knowledgeSourceType={getKnowledgeSourceType(
                                knowledgeSource
                            )}
                        />
                        {getKnowledgeSourceType(knowledgeSource) ===
                            KnowledgeSourceType.OneDrive ||
                        getKnowledgeSourceType(knowledgeSource) ===
                            KnowledgeSourceType.Website ? (
                            <a
                                href={
                                    sourceType === KnowledgeSourceType.Website
                                        ? knowledgeSource.websiteCrawlingConfig
                                              ?.urls?.[0]
                                        : knowledgeSource.onedriveConfig
                                              ?.sharedLinks?.[0]
                                }
                                target="_blank"
                                rel="noopener noreferrer"
                                className="hover:underline"
                            >
                                {getKnowledgeSourceDisplayName(knowledgeSource)}
                            </a>
                        ) : (
                            getKnowledgeSourceDisplayName(knowledgeSource)
                        )}
                    </div>
                    <div className="flex items-center mt-4">
                        <Button
                            variant="secondary"
                            onClick={onSyncNow}
                            tabIndex={-1}
                            className="w-[100px]"
                            disabled={
                                knowledgeSource.state ===
                                    KnowledgeSourceStatus.Syncing ||
                                knowledgeSource.state ===
                                    KnowledgeSourceStatus.Pending
                            }
                        >
                            Sync Now
                        </Button>
                        <Button
                            variant="secondary"
                            onClick={onDelete}
                            className="ml-2 items-center"
                        >
                            <Trash className="w-4 h-4 mr-2" />
                            Delete
                        </Button>
                    </div>
                </DialogTitle>
                <div className="flex flex-col gap-2 mt-2 max-h-96 w-1/2">
                    <div className="flex justify-between item-center h-[20px]">
                        <Label>Last Synced:</Label>
                        <Label>
                            {knowledgeSource.lastSyncEndTime
                                ? new Date(
                                      knowledgeSource.lastSyncEndTime
                                  ).toLocaleString()
                                : "Never"}
                        </Label>
                    </div>
                    <div className="flex justify-between items-center h-[20px]">
                        <Label>Duration:</Label>
                        <Label>
                            {knowledgeSource.lastSyncEndTime &&
                                knowledgeSource.lastSyncStartTime &&
                                (new Date(
                                    knowledgeSource.lastSyncEndTime
                                ).getTime() -
                                    new Date(
                                        knowledgeSource.lastSyncStartTime
                                    ).getTime()) /
                                    1000 +
                                    " seconds"}
                        </Label>
                    </div>
                    <div className="flex justify-between items-center h-[20px]">
                        <Label>Files Synced:</Label>
                        <Label>{files.length}</Label>
                    </div>
                    <div className="flex justify-between items-center h-[20px]">
                        <Label>Files added to Knowledge:</Label>
                        <Label>
                            {
                                files.filter(
                                    (file) =>
                                        file.state ===
                                        KnowledgeFileState.Ingested
                                ).length
                            }
                        </Label>
                    </div>
                    <div className="flex justify-between items-center h-[20px]">
                        <Label>Sync Schedule:</Label>
                        <DropdownMenu>
                            <DropdownMenuTrigger asChild>
                                <div className="flex items-center">
                                    <Button variant="ghost" size="icon">
                                        <EditIcon className="h-2 w-2" />
                                    </Button>
                                    <Label>
                                        {knowledgeSource.syncSchedule &&
                                        knowledgeSource.syncSchedule !== ""
                                            ? cronDescription
                                            : "On-Demand"}
                                    </Label>
                                </div>
                            </DropdownMenuTrigger>
                            <DropdownMenuContent className="w-[150px]">
                                <DropdownMenuItem
                                    className="cursor-pointer"
                                    onClick={() => {
                                        setSyncSchedule("");
                                        onSourceUpdate(
                                            "",
                                            autoApprove ?? false
                                        );
                                    }}
                                >
                                    On-Demand
                                    <DropdownMenuCheckboxItem
                                        checked={!knowledgeSource.syncSchedule}
                                    />
                                </DropdownMenuItem>
                                <DropdownMenuItem
                                    className="cursor-pointer"
                                    onClick={() => {
                                        setSyncSchedule("0 * * * *");
                                        onSourceUpdate(
                                            "0 * * * *",
                                            autoApprove ?? false
                                        );
                                    }}
                                >
                                    Hourly
                                    <DropdownMenuCheckboxItem
                                        checked={
                                            knowledgeSource.syncSchedule ===
                                            "0 * * * *"
                                        }
                                    />
                                </DropdownMenuItem>
                                <DropdownMenuItem
                                    className="cursor-pointer"
                                    onClick={() => {
                                        setSyncSchedule("0 0 * * *");
                                        onSourceUpdate(
                                            "0 0 * * *",
                                            autoApprove ?? false
                                        );
                                    }}
                                >
                                    Daily
                                    <DropdownMenuCheckboxItem
                                        checked={
                                            knowledgeSource.syncSchedule ===
                                            "0 0 * * *"
                                        }
                                    />
                                </DropdownMenuItem>
                                <DropdownMenuItem
                                    className="cursor-pointer"
                                    onClick={() => {
                                        setSyncSchedule("0 0 * * 0");
                                        onSourceUpdate(
                                            "0 0 * * 0",
                                            autoApprove ?? false
                                        );
                                    }}
                                >
                                    Weekly
                                    <DropdownMenuCheckboxItem
                                        checked={
                                            knowledgeSource.syncSchedule ===
                                            "0 0 * * 0"
                                        }
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
                                            ].includes(
                                                knowledgeSource.syncSchedule ??
                                                    ""
                                            ) && !!knowledgeSource.syncSchedule
                                        }
                                    />
                                </DropdownMenuItem>
                            </DropdownMenuContent>
                        </DropdownMenu>
                    </div>
                    <div className="flex justify-between items-center h-[20px]">
                        <Label>New Files Ingestion Policy:</Label>
                        <DropdownMenu>
                            <DropdownMenuTrigger asChild>
                                <div className="flex items-center">
                                    <Button variant="ghost" size="icon">
                                        <EditIcon className="h-2 w-2" />
                                    </Button>
                                    <Label className="flex-grow">
                                        {knowledgeSource.autoApprove
                                            ? "Automatic"
                                            : "Manual"}
                                    </Label>
                                </div>
                            </DropdownMenuTrigger>
                            <DropdownMenuContent className="w-[250px]">
                                <DropdownMenuItem
                                    className="cursor-pointer"
                                    onClick={() => {
                                        setAutoApprove(false);
                                        onSourceUpdate(
                                            syncSchedule ?? "",
                                            false
                                        );
                                    }}
                                >
                                    Manual
                                    <DropdownMenuCheckboxItem
                                        checked={!knowledgeSource.autoApprove}
                                    />
                                </DropdownMenuItem>
                                <DropdownMenuItem
                                    className="cursor-pointer"
                                    onClick={() => {
                                        setAutoApprove(true);
                                        onSourceUpdate(
                                            syncSchedule ?? "",
                                            true
                                        );
                                    }}
                                >
                                    Automatic
                                    <DropdownMenuCheckboxItem
                                        checked={knowledgeSource.autoApprove}
                                    />
                                </DropdownMenuItem>
                            </DropdownMenuContent>
                        </DropdownMenu>
                    </div>
                    <div className="flex justify-between items-center h-[20px]">
                        <Label>State:</Label>
                        {knowledgeSource.state ===
                        KnowledgeSourceStatus.Error ? (
                            <Tooltip>
                                <TooltipTrigger asChild>
                                    <Label className="flex items-center cursor-pointer text-destructive">
                                        <Button
                                            variant="ghost"
                                            onClick={() => {
                                                setErrorDialogError(
                                                    knowledgeSource.error ?? ""
                                                );
                                            }}
                                            className="items-center justify-center flex"
                                        >
                                            <span className="text-destructive">
                                                {knowledgeSource.state
                                                    ?.charAt(0)
                                                    .toUpperCase() +
                                                    knowledgeSource.state?.slice(
                                                        1
                                                    )}
                                            </span>
                                            <Eye className="w-4 h-4 text-destructive items-center justify-center" />
                                        </Button>
                                    </Label>
                                </TooltipTrigger>
                                <TooltipContent>View Error</TooltipContent>
                            </Tooltip>
                        ) : (
                            <Label className="flex items-center">
                                {knowledgeSource.state
                                    ?.charAt(0)
                                    .toUpperCase() +
                                    knowledgeSource.state?.slice(1)}
                                {knowledgeSource.state ===
                                    KnowledgeSourceStatus.Syncing && (
                                    <LoadingSpinner className="w-4 h-4 ml-2" />
                                )}
                            </Label>
                        )}
                    </div>
                    <div className="flex justify-between items-center h-[20px]">
                        <Label>Status:</Label>
                        {knowledgeSource.state ===
                            KnowledgeSourceStatus.Syncing && (
                            <div className="break-words text-gray-400 max-w-[800px]">
                                <Label>{knowledgeSource.status}</Label>
                            </div>
                        )}
                    </div>
                </div>

                <div className="mt-4 max-h-96">
                    <Table>
                        <TableHeader>
                            <TableRow>
                                <TableHead className="w-[15px]">
                                    <div className="flex justify-center items-center ">
                                        <AlertDialog>
                                            <AlertDialogTrigger asChild>
                                                <Button
                                                    variant="ghost"
                                                    size="icon"
                                                >
                                                    <Plus className="h-4 w-4" />
                                                </Button>
                                            </AlertDialogTrigger>
                                            <AlertDialogContent>
                                                <AlertDialogHeader>
                                                    <AlertDialogTitle>
                                                        Include All Files
                                                    </AlertDialogTitle>
                                                    <AlertDialogDescription>
                                                        This will immediately
                                                        ingest all files in the
                                                        knowledge base
                                                    </AlertDialogDescription>
                                                </AlertDialogHeader>
                                                <AlertDialogFooter>
                                                    <AlertDialogCancel>
                                                        Cancel
                                                    </AlertDialogCancel>
                                                    <AlertDialogAction
                                                        onClick={() => {
                                                            onApproveAllFiles(
                                                                true
                                                            );
                                                        }}
                                                    >
                                                        Continue
                                                    </AlertDialogAction>
                                                </AlertDialogFooter>
                                            </AlertDialogContent>
                                        </AlertDialog>
                                    </div>
                                </TableHead>
                                <TableHead>
                                    <div className="ml-2 flex flex-row items-center w-[700px]">
                                        <Button
                                            variant="ghost"
                                            size="sm"
                                            onClick={() => {
                                                setSortingOrder(
                                                    sortingColumn === "fileName"
                                                        ? sortingOrder === "asc"
                                                            ? "desc"
                                                            : "asc"
                                                        : "asc"
                                                );
                                                setSortingColumn("fileName");
                                            }}
                                        >
                                            <Label>Document</Label>
                                            {sortingOrder === "asc" ? (
                                                <ArrowUpDown
                                                    className="h-4 w-4"
                                                    strokeWidth={
                                                        sortingColumn ===
                                                        "fileName"
                                                            ? 2.5
                                                            : 1.5
                                                    }
                                                />
                                            ) : (
                                                <ArrowDownUp
                                                    className="h-4 w-4"
                                                    strokeWidth={
                                                        sortingColumn ===
                                                        "fileName"
                                                            ? 2.5
                                                            : 1.5
                                                    }
                                                />
                                            )}
                                        </Button>
                                    </div>
                                </TableHead>
                                <TableHead>
                                    <div className="flex items-center justify-center w-[50px]">
                                        <Button
                                            variant="ghost"
                                            size="sm"
                                            onClick={() => {
                                                setSortingColumn("state");
                                                setSortingOrder(
                                                    sortingColumn === "state"
                                                        ? sortingOrder === "asc"
                                                            ? "desc"
                                                            : "asc"
                                                        : "asc"
                                                );
                                            }}
                                        >
                                            <Label>State</Label>
                                            {sortingOrder === "asc" ? (
                                                <ArrowUpDown
                                                    className="h-4 w-4"
                                                    strokeWidth={
                                                        sortingColumn ===
                                                        "state"
                                                            ? 2.5
                                                            : 1.5
                                                    }
                                                />
                                            ) : (
                                                <ArrowDownUp
                                                    className="h-4 w-4"
                                                    strokeWidth={
                                                        sortingColumn ===
                                                        "state"
                                                            ? 2.5
                                                            : 1.5
                                                    }
                                                />
                                            )}
                                        </Button>
                                    </div>
                                </TableHead>
                                <TableHead>
                                    <div className="flex items-center justify-center">
                                        <Label>Ingestion Time</Label>
                                    </div>
                                </TableHead>
                                <TableHead>
                                    <div className="flex items-center justify-center">
                                        <Label>File Size</Label>
                                    </div>
                                </TableHead>
                            </TableRow>
                        </TableHeader>

                        <TableBody>
                            {files.map((file) => (
                                <TableRow key={file.id} className="group">
                                    <TableCell>
                                        <div className="flex justify-center items-center group">
                                            <Button
                                                variant="ghost"
                                                size="icon"
                                                onClick={() => {
                                                    const approved =
                                                        !file.approved;
                                                    onApproveFile(
                                                        file,
                                                        approved
                                                    );
                                                }}
                                                aria-label={
                                                    file.approved
                                                        ? "Disapprove"
                                                        : "Approve"
                                                }
                                                className="justify-center items-center"
                                            >
                                                {file.state ===
                                                KnowledgeFileState.Ingesting ? (
                                                    <LoadingSpinner className="w-4 h-4" />
                                                ) : file.state ===
                                                  KnowledgeFileState.Ingested ? (
                                                    <>
                                                        <CheckIcon className="w-4 h-4 text-success group-hover:hidden" />

                                                        <Tooltip>
                                                            <TooltipTrigger
                                                                asChild
                                                            >
                                                                <div className="flex justify-center items-center hidden group-hover:block">
                                                                    <MinusIcon className="w-4 h-4 text-danger" />
                                                                </div>
                                                            </TooltipTrigger>
                                                            <TooltipContent>
                                                                Exclude from
                                                                Knowledge
                                                            </TooltipContent>
                                                        </Tooltip>
                                                    </>
                                                ) : file.state ===
                                                  KnowledgeFileState.Pending ? (
                                                    <FileClock className="w-4 h-4" />
                                                ) : file.state ===
                                                  KnowledgeFileState.Error ? (
                                                    <>
                                                        <CircleX className="w-4 h-4 text-destructive group-hover:hidden" />

                                                        <Tooltip>
                                                            <TooltipTrigger
                                                                asChild
                                                            >
                                                                <div className="flex justify-center items-center hidden group-hover:block">
                                                                    <MinusIcon className="w-4 h-4 text-danger" />
                                                                </div>
                                                            </TooltipTrigger>
                                                            <TooltipContent>
                                                                Exclude from
                                                                Knowledge
                                                            </TooltipContent>
                                                        </Tooltip>
                                                    </>
                                                ) : file.state ===
                                                      KnowledgeFileState.PendingApproval ||
                                                  file.state ===
                                                      KnowledgeFileState.Unapproved ? (
                                                    <Tooltip>
                                                        <TooltipTrigger asChild>
                                                            <div className="flex justify-center items-center hidden group-hover:block">
                                                                <Plus className="w-4 h-4 text-danger" />
                                                            </div>
                                                        </TooltipTrigger>
                                                        <TooltipContent>
                                                            Add to Knowledge
                                                        </TooltipContent>
                                                    </Tooltip>
                                                ) : file.state ===
                                                  KnowledgeFileState.Unsupported ? (
                                                    <>
                                                        <ShieldAlert className="w-4 h-4 text-danger group-hover:hidden" />
                                                        <div className="flex justify-center items-center hidden group-hover:block">
                                                            <MinusIcon className="w-4 h-4 text-danger" />
                                                        </div>
                                                    </>
                                                ) : null}
                                            </Button>
                                        </div>
                                    </TableCell>
                                    <TableCell>
                                        <div
                                            className={`ml-2 w-[600px] group ${
                                                file.state ===
                                                    KnowledgeFileState.PendingApproval ||
                                                file.state ===
                                                    KnowledgeFileState.Unapproved
                                                    ? "text-gray-400"
                                                    : ""
                                            }`}
                                        >
                                            {renderFileElement(
                                                file,
                                                knowledgeSource
                                            )}
                                        </div>
                                    </TableCell>
                                    <TableCell>
                                        <div className="flex items-center justify-center w-[50px]">
                                            {file.state ===
                                                KnowledgeFileState.PendingApproval ||
                                            file.state ===
                                                KnowledgeFileState.Unapproved ? (
                                                <div className="flex justify-center">
                                                    <Label className="text-center text-gray-400">
                                                        Excluded
                                                    </Label>
                                                </div>
                                            ) : file.state ===
                                              KnowledgeFileState.Ingesting ? (
                                                <div className="flex justify-center items-center">
                                                    <Label>Ingesting</Label>
                                                </div>
                                            ) : file.state ===
                                              KnowledgeFileState.Pending ? (
                                                <div className="flex justify-center items-center">
                                                    <Label>Pending</Label>
                                                </div>
                                            ) : file.state ===
                                              KnowledgeFileState.Error ? (
                                                <div className="flex items-center justify-center group text-destructive">
                                                    <>
                                                        <Label className="text-destructive group-hover:hidden">
                                                            Error
                                                        </Label>

                                                        <Tooltip>
                                                            <TooltipTrigger
                                                                asChild
                                                            >
                                                                <Button
                                                                    variant="ghost"
                                                                    size="icon"
                                                                    className="hidden justify-center items-center group-hover:block text-destructive"
                                                                    onClick={async () => {
                                                                        await onReingestFile(
                                                                            file
                                                                        );
                                                                    }}
                                                                >
                                                                    <RefreshCcw className="h-4 w-4 text-destructive m-auto" />
                                                                </Button>
                                                            </TooltipTrigger>
                                                            <TooltipContent>
                                                                Reingest
                                                            </TooltipContent>
                                                        </Tooltip>
                                                        <Tooltip>
                                                            <TooltipTrigger
                                                                asChild
                                                            >
                                                                <Button
                                                                    variant="ghost"
                                                                    size="icon"
                                                                    className="hidden justify-center items-center group-hover:block text-destructive"
                                                                    onClick={() => {
                                                                        setErrorDialogError(
                                                                            file.error ??
                                                                                ""
                                                                        );
                                                                    }}
                                                                >
                                                                    <Eye className="h-4 w-4 text-destructive m-auto" />
                                                                </Button>
                                                            </TooltipTrigger>
                                                            <TooltipContent>
                                                                View Error
                                                            </TooltipContent>
                                                        </Tooltip>
                                                    </>
                                                </div>
                                            ) : file.state ===
                                              KnowledgeFileState.Ingested ? (
                                                <div className="flex justify-center items-center text-success">
                                                    <Label>Ingested</Label>
                                                </div>
                                            ) : file.state ===
                                              KnowledgeFileState.Unsupported ? (
                                                <div className="flex justify-center items-center">
                                                    <Tooltip>
                                                        <TooltipTrigger asChild>
                                                            <Label className="cursor-pointer">
                                                                Unsupported
                                                            </Label>
                                                        </TooltipTrigger>
                                                        <TooltipContent className="text-warning">
                                                            {file.error}
                                                        </TooltipContent>
                                                    </Tooltip>
                                                </div>
                                            ) : null}
                                        </div>
                                    </TableCell>
                                    <TableCell>
                                        <div className="flex items-center justify-center">
                                            {file.lastIngestionEndTime &&
                                            file.lastIngestionStartTime
                                                ? (new Date(
                                                      file.lastIngestionEndTime
                                                  ).getTime() -
                                                      new Date(
                                                          file.lastIngestionStartTime
                                                      ).getTime()) /
                                                      1000 +
                                                  " seconds"
                                                : ""}
                                        </div>
                                    </TableCell>
                                    <TableCell>
                                        <div className="flex items-center justify-center text-gray-400">
                                            {file.sizeInBytes
                                                ? file.sizeInBytes > 1000000
                                                    ? (
                                                          file.sizeInBytes /
                                                          1000000
                                                      ).toFixed(2) + " MB"
                                                    : file.sizeInBytes > 1000
                                                      ? (
                                                            file.sizeInBytes /
                                                            1000
                                                        ).toFixed(2) + " KB"
                                                      : file.sizeInBytes +
                                                        " Bytes"
                                                : ""}
                                        </div>
                                    </TableCell>
                                </TableRow>
                            ))}
                        </TableBody>
                    </Table>
                </div>

                <CronDialog
                    isOpen={isCronDialogOpen}
                    onOpenChange={setIsCronDialogOpen}
                    cronExpression={syncSchedule || ""}
                    setCronExpression={setSyncSchedule}
                    onSubmit={() => {
                        onSourceUpdate(
                            syncSchedule ?? "",
                            autoApprove ?? false
                        );
                    }}
                />
                <OauthSignDialog
                    agentId={agentId}
                    sourceType={sourceType}
                    knowledgeSource={knowledgeSource}
                />
                <ErrorDialog
                    error={errorDialogError}
                    isOpen={errorDialogError !== ""}
                    onClose={() => setErrorDialogError("")}
                />
            </DialogContent>
        </Dialog>
    );
};

export default KnowledgeSourceDetail;
