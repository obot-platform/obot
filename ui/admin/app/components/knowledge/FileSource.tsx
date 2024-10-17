import {
    CheckIcon,
    InfoIcon,
    PlusIcon,
    RefreshCcwIcon,
    SettingsIcon,
    XIcon,
} from "lucide-react";
import React from "react";

import {
    IngestionStatus,
    KnowledgeFile,
    KnowledgeIngestionStatus,
    RemoteKnowledgeSource,
    RemoteKnowledgeSourceType,
    getMessage,
    getRemoteFileDisplayName,
} from "~/lib/model/knowledge";
import { KnowledgeService } from "~/lib/service/api/knowledgeService";
import { cn } from "~/lib/utils";

import { Button } from "~/components/ui/button";
import { ScrollArea } from "~/components/ui/scroll-area";
import {
    Tooltip,
    TooltipContent,
    TooltipProvider,
    TooltipTrigger,
} from "~/components/ui/tooltip";
import { useAsync } from "~/hooks/useAsync";

import { LoadingSpinner } from "../ui/LoadingSpinner";
import { AddFileModal } from "./AddFileModal";
import { FileChip } from "./FileItem";
import IngestionStatusComponent from "./IngestionStatus";
import RemoteFileAvatar from "./RemoteFileAvatar";
import RemoteFileItemChip from "./RemoteFileItemChip";
import RemoteKnowledgeSourceStatus from "./RemoteKnowledgeSourceStatus";
import RemoteSourceSettingModal from "./RemoteSourceSettingModal";
import { NotionModal } from "./notion/NotionModal";
import { OnedriveModal } from "./onedrive/OneDriveModal";
import { WebsiteModal } from "./website/WebsiteModal";

interface FileSourceProps {
    agentId: string;
    remoteKnowledgeSources: RemoteKnowledgeSource[];
    knowledge: KnowledgeFile[];
    fileInputRef: React.RefObject<HTMLInputElement>;
    isAddFileModalOpen: boolean;
    onAddFileModalOpen: (open: boolean) => void;
    startPolling: () => void;
    getKnowledge: any;
    getRemoteKnowledgeSources: any;
}

const FileSource = ({
    agentId,
    remoteKnowledgeSources,
    knowledge,
    getKnowledge,
    fileInputRef,
    isAddFileModalOpen,
    onAddFileModalOpen,
    getRemoteKnowledgeSources,
    startPolling,
}: FileSourceProps) => {
    const [isOnedriveModalOpen, setIsOnedriveModalOpen] = React.useState(false);
    const [isNotionModalOpen, setIsNotionModalOpen] = React.useState(false);
    const [isWebsiteModalOpen, setIsWebsiteModalOpen] = React.useState(false);

    const [isRemoteSourceSettingModalOpen, setIsRemoteSourceSettingModalOpen] =
        React.useState(false);
    const [selectedRemoteKnowledgeSource, setSelectedRemoteKnowledgeSource] =
        React.useState<RemoteKnowledgeSource | null>(null);

    const deleteKnowledge = useAsync(async (item: KnowledgeFile) => {
        await KnowledgeService.deleteKnowledgeFromAgent(agentId, item.fileName);

        const remoteKnowledgeSource = remoteKnowledgeSources?.find(
            (source) => source.sourceType === item.remoteKnowledgeSourceType
        );
        if (remoteKnowledgeSource) {
            await KnowledgeService.updateRemoteKnowledgeSource(
                agentId,
                remoteKnowledgeSource.id,
                {
                    ...remoteKnowledgeSource,
                    exclude: [
                        ...(remoteKnowledgeSource.exclude || []),
                        item.uploadID || "",
                    ],
                }
            );
        }

        // optomistic update without cache revalidation
        getKnowledge.mutate((prev: KnowledgeFile[]) =>
            prev?.filter((prevItem) => prevItem.fileName !== item.fileName)
        );
    });

    const handleRemoteKnowledgeSourceSync = async (
        knowledgeSourceType: RemoteKnowledgeSourceType
    ) => {
        try {
            const source = remoteKnowledgeSources?.find(
                (source) => source.sourceType === knowledgeSourceType
            );
            if (source) {
                await KnowledgeService.resyncRemoteKnowledgeSource(
                    agentId,
                    source.id
                );
            }
            const intervalId = setInterval(() => {
                getRemoteKnowledgeSources.mutate();
                const updatedSource = remoteKnowledgeSources?.find(
                    (source) => source.sourceType === knowledgeSourceType
                );
                if (updatedSource?.runID) {
                    clearInterval(intervalId);
                }
            }, 1000);
            // this is a failsafe to clear the interval as source should be updated with runID in 10 seconds once the source is resynced
            setTimeout(() => {
                clearInterval(intervalId);
                startPolling();
            }, 10000);
        } catch (error) {
            console.error("Failed to resync remote knowledge source:", error);
        }
    };

    const handleRemoteSourceSetting = (source: RemoteKnowledgeSource) => {
        setSelectedRemoteKnowledgeSource(source);
        setIsRemoteSourceSettingModalOpen(true);
    };
    return (
        <>
            {[
                "files",
                ...new Set(
                    remoteKnowledgeSources
                        .map((source) => source.sourceType!)
                        .sort((a, b) => a.localeCompare(b))
                ),
            ].map((type) => (
                <div key={type} className="mb-2">
                    <div className="flex justify-between items-center mb-2">
                        <div className="flex items-center space-x-2">
                            <RemoteFileAvatar
                                remoteKnowledgeSourceType={
                                    type as RemoteKnowledgeSourceType
                                }
                            />
                            <h3 className="text-lg font-semibold">
                                {type.charAt(0).toUpperCase() + type.slice(1)}
                            </h3>
                        </div>
                        <div className="flex space-x-2">
                            <Button
                                size="sm"
                                variant="secondary"
                                onClick={async () => {
                                    if (type === "notion") {
                                        const notionSource =
                                            remoteKnowledgeSources.find(
                                                (source) =>
                                                    source.sourceType ===
                                                    "notion"
                                            );
                                        setIsNotionModalOpen(true);
                                        await KnowledgeService.resyncRemoteKnowledgeSource(
                                            agentId,
                                            notionSource?.id!
                                        );
                                    } else if (type === "onedrive") {
                                        const onedriveSource =
                                            remoteKnowledgeSources.find(
                                                (source) =>
                                                    source.sourceType ===
                                                    "onedrive"
                                            );
                                        setIsOnedriveModalOpen(true);
                                        await KnowledgeService.resyncRemoteKnowledgeSource(
                                            agentId,
                                            onedriveSource?.id!
                                        );
                                    } else if (type === "website") {
                                        setIsWebsiteModalOpen(true);
                                    } else {
                                        fileInputRef.current?.click();
                                    }
                                }}
                            >
                                <PlusIcon className="w-4 h-4" />
                            </Button>
                            {type !== "files" && (
                                <>
                                    <Button
                                        size="sm"
                                        variant="secondary"
                                        onClick={() =>
                                            handleRemoteKnowledgeSourceSync(
                                                type as RemoteKnowledgeSourceType
                                            )
                                        }
                                    >
                                        <RefreshCcwIcon className="w-4 h-4" />
                                    </Button>
                                    <Button
                                        size="sm"
                                        variant="secondary"
                                        onClick={() =>
                                            handleRemoteSourceSetting(
                                                remoteKnowledgeSources.find(
                                                    (source) =>
                                                        source.sourceType ===
                                                        type
                                                )!
                                            )
                                        }
                                    >
                                        <SettingsIcon className="w-4 h-4" />
                                    </Button>
                                </>
                            )}
                        </div>
                    </div>
                    <div className="border-b border-gray-200 mb-4">
                        <ScrollArea className="max-h-[200px]">
                            <div className={cn("p-2 flex flex-wrap gap-2")}>
                                {type === "website"
                                    ? Object.entries(
                                          knowledge
                                              .filter(
                                                  (item) =>
                                                      (item.remoteKnowledgeSourceType ||
                                                          "files") === type
                                              )
                                              .reduce(
                                                  (acc, item) => {
                                                      const source =
                                                          remoteKnowledgeSources.find(
                                                              (source) =>
                                                                  source.sourceType ===
                                                                  type
                                                          );
                                                      const parentUrl =
                                                          source?.state
                                                              .websiteCrawlingState
                                                              ?.pages?.[
                                                              item.fileDetails
                                                                  .url!
                                                          ]?.parentUrl ||
                                                          "Other";
                                                      if (!acc[parentUrl]) {
                                                          acc[parentUrl] = [];
                                                      }
                                                      acc[parentUrl].push(item);
                                                      return acc;
                                                  },
                                                  {} as Record<
                                                      string,
                                                      (typeof knowledge)[0][]
                                                  >
                                              )
                                      ).map(([parentUrl, items]) => (
                                          <div
                                              key={parentUrl}
                                              className="w-full flex flex-col gap-2"
                                          >
                                              <h4 className="text-base font-medium mb-1">
                                                  {parentUrl}
                                              </h4>
                                              {items.map((item) => (
                                                  <RemoteFileItemChip
                                                      key={item.fileName}
                                                      url={
                                                          item.fileDetails.url!
                                                      }
                                                      displayName={
                                                          getRemoteFileDisplayName(
                                                              item
                                                          )!
                                                      }
                                                      onAction={() =>
                                                          deleteKnowledge.execute(
                                                              item
                                                          )
                                                      }
                                                      statusIcon={renderStatusIcon(
                                                          item.ingestionStatus
                                                      )}
                                                      isLoading={
                                                          deleteKnowledge.isLoading &&
                                                          deleteKnowledge
                                                              .lastCallParams?.[0]
                                                              .fileName ===
                                                              item.fileName
                                                      }
                                                      remoteKnowledgeSourceType={
                                                          item.remoteKnowledgeSourceType!
                                                      }
                                                  />
                                              ))}
                                          </div>
                                      ))
                                    : knowledge
                                          .filter(
                                              (item) =>
                                                  (item.remoteKnowledgeSourceType ||
                                                      "files") === type
                                          )
                                          .map((item) => {
                                              if (
                                                  item.remoteKnowledgeSourceType
                                              ) {
                                                  return (
                                                      <RemoteFileItemChip
                                                          key={item.fileName}
                                                          url={
                                                              item.fileDetails
                                                                  .url!
                                                          }
                                                          displayName={
                                                              getRemoteFileDisplayName(
                                                                  item
                                                              )!
                                                          }
                                                          onAction={() =>
                                                              deleteKnowledge.execute(
                                                                  item
                                                              )
                                                          }
                                                          statusIcon={renderStatusIcon(
                                                              item.ingestionStatus
                                                          )}
                                                          isLoading={
                                                              deleteKnowledge.isLoading &&
                                                              deleteKnowledge
                                                                  .lastCallParams?.[0]
                                                                  .fileName ===
                                                                  item.fileName
                                                          }
                                                          remoteKnowledgeSourceType={
                                                              item.remoteKnowledgeSourceType
                                                          }
                                                      />
                                                  );
                                              }
                                              return (
                                                  <FileChip
                                                      key={item.fileName}
                                                      onAction={() =>
                                                          deleteKnowledge.execute(
                                                              item
                                                          )
                                                      }
                                                      statusIcon={renderStatusIcon(
                                                          item.ingestionStatus
                                                      )}
                                                      isLoading={
                                                          deleteKnowledge.isLoading &&
                                                          deleteKnowledge
                                                              .lastCallParams?.[0]
                                                              .fileName ===
                                                              item.fileName
                                                      }
                                                      fileName={item.fileName}
                                                  />
                                              );
                                          })}
                            </div>
                        </ScrollArea>
                        <div className="flex flex-col items-start mb-4">
                            <IngestionStatusComponent
                                knowledge={knowledge.filter(
                                    (item) =>
                                        (item.remoteKnowledgeSourceType ||
                                            "files") === type
                                )}
                            />

                            {type !== "files" && (
                                <RemoteKnowledgeSourceStatus
                                    source={
                                        remoteKnowledgeSources.find(
                                            (source) =>
                                                source.sourceType === type
                                        )!
                                    }
                                />
                            )}
                        </div>
                    </div>
                </div>
            ))}
            <RemoteSourceSettingModal
                agentId={agentId}
                isOpen={isRemoteSourceSettingModalOpen}
                onOpenChange={setIsRemoteSourceSettingModalOpen}
                remoteKnowledgeSource={selectedRemoteKnowledgeSource}
            />
            <AddFileModal
                agentId={agentId}
                isOpen={isAddFileModalOpen}
                onOpenChange={onAddFileModalOpen}
                remoteKnowledgeSources={remoteKnowledgeSources}
                onWebsiteModalOpen={setIsWebsiteModalOpen}
                onNotionModalOpen={setIsNotionModalOpen}
                onOneDriveModalOpen={setIsOnedriveModalOpen}
                getRemoteKnowledgeSources={getRemoteKnowledgeSources}
            />
            <NotionModal
                agentId={agentId}
                isOpen={isNotionModalOpen}
                onOpenChange={setIsNotionModalOpen}
                remoteKnowledgeSources={remoteKnowledgeSources}
                startPolling={startPolling}
            />
            <OnedriveModal
                agentId={agentId}
                isOpen={isOnedriveModalOpen}
                onOpenChange={setIsOnedriveModalOpen}
                remoteKnowledgeSources={remoteKnowledgeSources}
                startPolling={startPolling}
                getRemoteKnowledgeSources={getRemoteKnowledgeSources}
            />
            <WebsiteModal
                agentId={agentId}
                isOpen={isWebsiteModalOpen}
                onOpenChange={setIsWebsiteModalOpen}
                remoteKnowledgeSources={remoteKnowledgeSources}
                startPolling={startPolling}
                getRemoteKnowledgeSources={getRemoteKnowledgeSources}
            />
        </>
    );
};

function renderStatusIcon(status?: KnowledgeIngestionStatus) {
    if (!status || !status.status) return null;
    const [Icon, className] = ingestionIcons[status.status];

    return (
        <TooltipProvider>
            <Tooltip>
                <TooltipTrigger asChild>
                    <div>
                        {Icon === LoadingSpinner ? (
                            <LoadingSpinner
                                className={cn("w-4 h-4", className)}
                            />
                        ) : (
                            <Icon className={cn("w-4 h-4", className)} />
                        )}
                    </div>
                </TooltipTrigger>
                <TooltipContent className="whitespace-normal break-words max-w-[300px] max-h-full">
                    {getMessage(status.status, status.msg, status.error)}
                </TooltipContent>
            </Tooltip>
        </TooltipProvider>
    );
}

const ingestionIcons = {
    [IngestionStatus.Queued]: [LoadingSpinner, ""],
    [IngestionStatus.Finished]: [CheckIcon, "text-green-500"],
    [IngestionStatus.Completed]: [LoadingSpinner, ""],
    [IngestionStatus.Skipped]: [CheckIcon, "text-green-500"],
    [IngestionStatus.Starting]: [LoadingSpinner, ""],
    [IngestionStatus.Failed]: [XIcon, "text-destructive"],
    [IngestionStatus.Unsupported]: [InfoIcon, "text-yellow-500"],
} as const;

export default FileSource;
