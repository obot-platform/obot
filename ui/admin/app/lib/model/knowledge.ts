export const IngestionStatus = {
    Queued: "queued",
    Completed: "completed",
    Finished: "finished",
    Skipped: "skipped",
    Failed: "failed",
    Starting: "starting",
    Unsupported: "unsupported",
} as const;
export type IngestionStatus =
    (typeof IngestionStatus)[keyof typeof IngestionStatus];

export const RemoteKnowledgeSourceType = {
    OneDrive: "onedrive",
    Notion: "notion",
    Website: "website",
} as const;
export type RemoteKnowledgeSourceType =
    (typeof RemoteKnowledgeSourceType)[keyof typeof RemoteKnowledgeSourceType];

export type KnowledgeIngestionStatus = {
    count?: number;
    reason?: string;
    absolute_path?: string;
    basePath?: string;
    filename?: string;
    vectorstore?: string;
    msg?: string;
    flow?: string;
    rootPath?: string;
    filepath?: string;
    phase?: string;
    num_documents?: number;
    stage?: string;
    status?: IngestionStatus;
    component?: string;
    filetype?: string;
    error?: string;
};

export type RemoteKnowledgeSource = {
    id: string;
    runID?: string;
    threadID?: string;
    status?: string;
    error?: string;
    state: RemoteKnowledgeSourceState;
} & RemoteKnowledgeSourceInput;

export type RemoteKnowledgeSourceInput = {
    syncSchedule?: string;
    sourceType?: RemoteKnowledgeSourceType;
    autoApprove?: boolean;
    onedriveConfig?: OneDriveConfig;
    notionConfig?: NotionConfig;
    websiteCrawlingConfig?: WebsiteCrawlingConfig;
};

type OneDriveConfig = {
    sharedLinks?: string[];
};

type NotionConfig = {
    pages?: string[];
};

type WebsiteCrawlingConfig = {
    urls?: string[];
};

type RemoteKnowledgeSourceState = {
    onedriveState?: OneDriveLinksConnectorState;
    notionState?: NotionConnectorState;
    websiteCrawlingState?: WebsiteCrawlingConnectorState;
};

type OneDriveLinksConnectorState = {
    folders?: FolderSet;
    files?: Record<string, FileState>;
    links?: Record<string, LinkState>;
};

type LinkState = {
    name?: string;
    isFolder?: boolean;
};

type FileState = {
    fileName: string;
    folderPath?: string;
    url?: string;
};

type NotionConnectorState = {
    pages?: Record<string, NotionPage>;
};

type NotionPage = {
    url?: string;
    title?: string;
    folderPath?: string;
};

type WebsiteCrawlingConnectorState = {
    pages?: Record<string, PageDetails>;
    scrapeJobIds?: Record<string, string>;
    folders?: FolderSet;
};

type PageDetails = {
    parentUrl?: string;
};

type FolderSet = {
    [key: string]: undefined;
};

type FileDetails = {
    filePath?: string;
    url?: string;
    updatedAd?: string; // date
};

export type KnowledgeFile = {
    id?: string;
    deleted?: string;
    fileName: string;
    agentID?: string;
    workflowName?: string;
    threadName?: string;
    remoteKnowledgeSourceType?: RemoteKnowledgeSourceType;
    remoteKnowledgeSourceID?: string;
    ingestionStatus: KnowledgeIngestionStatus;
    fileDetails: FileDetails;
    uploadID?: string;
    approved?: boolean;
};

export function getRemoteFileDisplayName(item: KnowledgeFile) {
    if (item.remoteKnowledgeSourceType === RemoteKnowledgeSourceType.OneDrive) {
        return item.fileName.split("/").pop()!;
    }
    if (item.remoteKnowledgeSourceType === RemoteKnowledgeSourceType.Notion) {
        return item.fileName.split("/").pop()!.replace(/\.md$/, "");
    }
    if (item.remoteKnowledgeSourceType === RemoteKnowledgeSourceType.Website) {
        return item.fileDetails.url;
    }
    return item.fileName;
}

export function getIngestionStatus(
    status?: KnowledgeIngestionStatus
): IngestionStatus {
    if (!status) {
        return IngestionStatus.Queued;
    }

    if (
        status.status === IngestionStatus.Skipped &&
        status.reason === "unsupported"
    ) {
        return IngestionStatus.Unsupported;
    }

    return status.status || IngestionStatus.Queued;
}

export function getMessage(
    status?: IngestionStatus,
    msg?: string,
    error?: string
) {
    if (!status) return "Queued";

    if (
        status === IngestionStatus.Finished ||
        status === IngestionStatus.Skipped
    ) {
        return "Exclude file from ingestion";
    }

    if (status === IngestionStatus.Failed) {
        return error || msg || "Failed";
    }

    if (status === IngestionStatus.Unsupported) {
        return "This file type is not supported for ingestion.";
    }

    return msg || "Queued";
}
