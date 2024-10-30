export const RemoteKnowledgeSourceType = {
    OneDrive: "onedrive",
    Notion: "notion",
    Website: "website",
} as const;
export type RemoteKnowledgeSourceType =
    (typeof RemoteKnowledgeSourceType)[keyof typeof RemoteKnowledgeSourceType];

export const KnowledgeFileState = {
    Pending: "pending",
    Ingesting: "ingesting",
    Ingested: "ingested",
    Error: "error",
    Unapproved: "unapproved",
    PendingApproval: "pending-approval",
} as const;
export type KnowledgeFileState =
    (typeof KnowledgeFileState)[keyof typeof KnowledgeFileState];

export const KnowledgeSourceStatus = {
    Pending: "pending",
    Syncing: "syncing",
    Synced: "synced",
    Error: "error",
} as const;
export type KnowledgeSourceStatus =
    (typeof KnowledgeSourceStatus)[keyof typeof KnowledgeSourceStatus];

export type KnowledgeSource = {
    id: string;
    agentID: string;
    state?: KnowledgeSourceStatus;
    syncDetails?: RemoteKnowledgeSourceState;
    status?: string;
    error?: string;
    authStatus?: AuthStatus;
} & KnowledgeSourceInput;

type AuthStatus = {
    url?: string;
    authenticated?: boolean;
    required?: boolean;
    error?: string;
};

export type KnowledgeSourceInput = {
    syncSchedule?: string;
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
    parentURL?: string;
};

type FolderSet = {
    [key: string]: undefined;
};

export type KnowledgeFile = {
    id: string;
    created: string;
    fileName: string;
    state: KnowledgeFileState;
    agentID: string;
    knowledgeSetID: string;
    knowledgeSourceID: string;
    url: string;
    updatedAt: string;
    checksum: string;
    approved: boolean;
    lastRunID: string;
};

export function getRemoteFileDisplayName(item: KnowledgeFile) {
    return item.fileName;
}

export function getMessage(state: KnowledgeFileState, error?: string) {
    if (state === KnowledgeFileState.Error) {
        return error ?? "Ingestion failed";
    }

    if (state === KnowledgeFileState.PendingApproval) {
        return "Pending approval, click to approve";
    }

    return state;
}
