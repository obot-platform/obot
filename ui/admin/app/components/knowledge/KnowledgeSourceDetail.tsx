import cronstrue from "cronstrue";
import { EditIcon, Eye, InfoIcon, Trash } from "lucide-react";
import { FC, useCallback, useEffect, useMemo, useRef, useState } from "react";
import useSWR, { SWRResponse } from "swr";

import {
    KnowledgeFile,
    KnowledgeFileState,
    KnowledgeSource,
    KnowledgeSourceStatus,
    KnowledgeSourceType,
    getKnowledgeFilePathNameForFileTree,
    getKnowledgeSourceDisplayName,
    getKnowledgeSourceType,
} from "~/lib/model/knowledge";
import { KnowledgeService } from "~/lib/service/api/knowledgeService";

import CronDialog from "~/components/knowledge/CronDialog";
import ErrorDialog from "~/components/knowledge/ErrorDialog";
import FileTreeNode, { FileNode } from "~/components/knowledge/FileTree";
import KnowledgeSourceAvatar from "~/components/knowledge/KnowledgeSourceAvatar";
import OauthSignDialog from "~/components/knowledge/OAuthSignDialog";
import { LoadingSpinner } from "~/components/ui/LoadingSpinner";
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
    const [isCronDialogOpen, setIsCronDialogOpen] = useState(false);
    const [cronDescription, setCronDescription] = useState("");

    const [errorDialogError, setErrorDialogError] = useState("");
    const sourceType = getKnowledgeSourceType(knowledgeSource);

    const tableContainerRef = useRef<HTMLDivElement>(null);
    const scrollPosition = useRef(0);

    useEffect(() => {
        setSyncSchedule(knowledgeSource.syncSchedule);
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

    const files = useMemo(
        () =>
            getFiles.data?.sort((a, b) =>
                a.fileName.localeCompare(b.fileName)
            ) ?? [],
        [getFiles.data]
    );

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

    const onSourceUpdate = async (syncSchedule: string) => {
        const updatedSource = await KnowledgeService.updateKnowledgeSource(
            agentId,
            knowledgeSource.id,
            {
                ...knowledgeSource,
                syncSchedule: syncSchedule,
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

    const onApproveFileNode = async (approved: boolean, fileNode: FileNode) => {
        if (fileNode.file) {
            try {
                await onApproveFile(fileNode.file, approved);
            } catch (e) {
                console.error("failed to approve file", fileNode.file, e);
            }
            return;
        }
        if (fileNode.children) {
            for (const child of fileNode.children) {
                await onApproveFileNode(approved, child);
            }
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

    const constructFileTree = useCallback(
        (files: KnowledgeFile[]): FileNode[] => {
            const roots: FileNode[] = [];

            function addPathToTree(
                parts: string[],
                file: KnowledgeFile,
                currentNode: FileNode
            ) {
                if (parts.length === 0) return;

                const currentPart = parts[0];
                const isFile = parts.length === 1;
                let childNode = currentNode.children?.find(
                    (child) => child.name === currentPart
                );

                if (!childNode) {
                    childNode = {
                        name: currentPart,
                        file: isFile ? file : null,
                        children: isFile ? [] : [],
                        path: currentNode.path + currentPart + "/",
                    };
                    currentNode.children?.push(childNode);
                }

                addPathToTree(parts.slice(1), file, childNode);
            }

            for (const file of files) {
                const pathName = getKnowledgeFilePathNameForFileTree(
                    file,
                    knowledgeSource
                );
                const pathParts = pathName.split("/");
                let root = roots.find((r) => r.name === pathParts[0]);
                if (!root) {
                    root = {
                        name: pathParts[0],
                        file: null,
                        children: [],
                        path: pathParts[0] + "/",
                    };
                    if (pathParts.length === 1) {
                        root.file = file;
                        root.path = pathParts[0];
                    }
                    roots.push(root);
                }
                addPathToTree(pathParts.slice(1), file, root);
            }

            return roots.sort((a, b) => {
                if (a.file === null && b.file !== null) return -1;
                if (a.file !== null && b.file === null) return 1;
                return a.path.localeCompare(b.path);
            });
        },
        [knowledgeSource]
    );

    const fileNodes = useMemo(() => {
        return constructFileTree(files);
    }, [files, constructFileTree]);

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
                        <Label className="flex items-center">
                            {files.length}
                            {sourceType === KnowledgeSourceType.Website &&
                                files.length >= 250 && (
                                    <Tooltip>
                                        <TooltipTrigger asChild>
                                            <Button
                                                variant="ghost"
                                                size="sm"
                                                className="h-2 w-2"
                                            >
                                                <InfoIcon className="h-2 w-2" />
                                            </Button>
                                        </TooltipTrigger>
                                        <TooltipContent>
                                            You have reached the maximum number
                                            of files that can be synced
                                        </TooltipContent>
                                    </Tooltip>
                                )}
                        </Label>
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
                                        onSourceUpdate("");
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
                                        onSourceUpdate("0 * * * *");
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
                                        onSourceUpdate("0 0 * * *");
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
                                        onSourceUpdate("0 0 * * 0");
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
                    <div className="flex justify-between items-center">
                        <Label>Status:</Label>
                        {knowledgeSource.state ===
                            KnowledgeSourceStatus.Syncing && (
                            <div className="ml-4 break-words text-gray-400 overflow-y-auto truncate">
                                <Tooltip>
                                    <TooltipTrigger asChild>
                                        <Label className="cursor-pointer">
                                            {knowledgeSource.status}
                                        </Label>
                                    </TooltipTrigger>
                                    <TooltipContent>
                                        {knowledgeSource.status}
                                    </TooltipContent>
                                </Tooltip>
                            </div>
                        )}
                    </div>
                </div>
                <div className="flex flex-col gap-2 mt-2 max-h-96">
                    {fileNodes.map((node) => (
                        <FileTreeNode
                            key={node.path}
                            node={node}
                            level={0}
                            source={knowledgeSource}
                            onApproveFile={onApproveFileNode}
                            onReingestFile={onReingestFile}
                            setErrorDialogError={setErrorDialogError}
                            updateKnowledgeSource={async (source) => {
                                const res =
                                    await KnowledgeService.updateKnowledgeSource(
                                        agentId,
                                        knowledgeSource.id,
                                        source
                                    );
                                onSave(res);
                            }}
                        />
                    ))}
                </div>

                <CronDialog
                    isOpen={isCronDialogOpen}
                    onOpenChange={setIsCronDialogOpen}
                    cronExpression={syncSchedule || ""}
                    setCronExpression={setSyncSchedule}
                    onSubmit={() => {
                        onSourceUpdate(syncSchedule ?? "");
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
