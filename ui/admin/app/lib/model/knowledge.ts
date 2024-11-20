export const KnowledgeSourceType = {
    OneDrive: "OneDrive",
    Notion: "Notion",
    Website: "Website",
} as const;
export type KnowledgeSourceType =
    (typeof KnowledgeSourceType)[keyof typeof KnowledgeSourceType];

export const KnowledgeFileState = {
    Pending: "pending",
    Ingesting: "ingesting",
    Ingested: "ingested",
    Error: "error",
    Unapproved: "unapproved",
    PendingApproval: "pending-approval",
    Unsupported: "unsupported",
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
    name: string;
    agentID: string;
    state: KnowledgeSourceStatus;
    syncDetails?: RemoteKnowledgeSourceState;
    status?: string;
    error?: string;
    authStatus?: AuthStatus;
    lastSyncStartTime?: string;
    lastSyncEndTime?: string;
    lastRunID?: string;
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
    filePathPrefixInclude?: string[];
    filePathPrefixExclude?: string[];
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
    fileName: string;
    state: KnowledgeFileState;
    error?: string;
    agentID?: string;
    threadID?: string;
    knowledgeSetID?: string;
    knowledgeSourceID?: string;
    approved?: boolean;
    url?: string;
    updatedAt?: string;
    checksum?: string;
    lastIngestionStartTime?: Date;
    lastIngestionEndTime?: Date;
    lastRunIDs?: string[];
    deleted?: string;
    sizeInBytes?: number;
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

export function getKnowledgeSourceType(source: KnowledgeSource) {
    if (source.notionConfig) {
        return KnowledgeSourceType.Notion;
    }

    if (source.onedriveConfig) {
        return KnowledgeSourceType.OneDrive;
    }

    return KnowledgeSourceType.Website;
}

export function getKnowledgeSourceDisplayName(source: KnowledgeSource) {
    if (source.notionConfig) {
        return "Notion";
    }

    if (source.onedriveConfig) {
        if (
            source.syncDetails?.onedriveState?.links &&
            source.onedriveConfig.sharedLinks &&
            source.onedriveConfig.sharedLinks.length > 0
        ) {
            return source.syncDetails?.onedriveState?.links[
                source.onedriveConfig.sharedLinks[0]
            ].name;
        }

        return "OneDrive";
    }

    if (source.websiteCrawlingConfig) {
        if (
            source.websiteCrawlingConfig.urls &&
            source.websiteCrawlingConfig.urls.length > 0
        ) {
            return source.websiteCrawlingConfig.urls[0];
        }

        return "Website";
    }

    return source.name;
}

export function getToolRefForKnowledgeSource(sourceType: KnowledgeSourceType) {
    if (sourceType === KnowledgeSourceType.OneDrive) {
        return "onedrive-data-source";
    }

    if (sourceType === KnowledgeSourceType.Notion) {
        return "notion-data-source";
    }

    if (sourceType === KnowledgeSourceType.Website) {
        return "website-data-source";
    }

    return "";
}

export function getKnowledgeFileDisplayName(
    file: KnowledgeFile,
    source: KnowledgeSource
) {
    let displayName = file.fileName;
    let subTitle;
    const sourceType = getKnowledgeSourceType(source);
    if (sourceType === KnowledgeSourceType.Notion) {
        displayName = file.fileName.split("/").pop()!;
        subTitle =
            source.syncDetails?.notionState?.pages?.[file.url!]?.folderPath;
    } else if (sourceType === KnowledgeSourceType.OneDrive) {
        const parts = file.fileName.split("/");
        displayName = parts.pop()!;
        subTitle = parts.join("/");
    } else if (sourceType === KnowledgeSourceType.Website) {
        displayName = file.url ?? "";
    }
    return { displayName, subTitle };
}

export function getKnowledgeFilePathNameForFileTree(
    file: KnowledgeFile,
    source: KnowledgeSource
) {
    const sourceType = getKnowledgeSourceType(source);
    if (sourceType === KnowledgeSourceType.Notion) {
        // For Notion, we need to remove the last folder from the path as it is usually page ID which is not useful to user
        // The reason we have ID in the path to make sure we can uniquely identify the file because file name can be same for different pages
        const parts = file.fileName.split("/");
        if (parts.length > 2) {
            parts.splice(-2, 1);
        } else if (parts.length === 2) {
            return parts[1];
        }
        return parts.join("/");
    }

    return file.fileName.replace(/^\//, "");
}
