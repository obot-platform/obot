import { useEffect, useMemo, useState } from "react";
import useSWR from "swr";

import {
    KnowledgeFile,
    KnowledgeFileState,
    KnowledgeSource,
    KnowledgeSourceStatus,
} from "~/lib/model/knowledge";
import { KnowledgeService } from "~/lib/service/api/knowledgeService";
import { handlePromise } from "~/lib/service/async";

export function useKnowledgeSourceFiles(
    agentId: string,
    knowledgeSource: KnowledgeSource
) {
    const [blockPollingFiles, setBlockPollingFiles] = useState(true);

    const startPolling = () => {
        if (blockPollingFiles) setBlockPollingFiles(false);
    };

    if (
        knowledgeSource.state === KnowledgeSourceStatus.Syncing ||
        knowledgeSource.state === KnowledgeSourceStatus.Pending
    ) {
        startPolling();
    }

    const {
        data: files,
        mutate: mutateFiles,
        ...rest
    } = useSWR(
        KnowledgeService.getFilesForKnowledgeSource.key(
            agentId,
            knowledgeSource.id
        ),
        ({ agentId, sourceId }) =>
            KnowledgeService.getFilesForKnowledgeSource(agentId, sourceId),
        {
            revalidateOnFocus: false,
            refreshInterval: blockPollingFiles ? undefined : 5000,
        }
    );

    const sortedFiles = useMemo(() => {
        return (
            files?.sort((a, b) => a.fileName.localeCompare(b.fileName)) ?? []
        );
    }, [files]);

    useEffect(() => {
        if (sortedFiles.length === 0) {
            setBlockPollingFiles(true);
            return;
        }

        if (
            sortedFiles
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
    }, [sortedFiles]);

    const reingestFile = async (fileId: string) => {
        const updatedFile = await KnowledgeService.reingestFile(
            agentId,
            fileId,
            knowledgeSource.id
        );
        mutateFiles((prev) =>
            prev?.map((f) => (f.id === fileId ? updatedFile : f))
        );
    };

    const approveFile = async (file: KnowledgeFile, approved: boolean) => {
        const { error, data: updatedFile } = await handlePromise(
            KnowledgeService.approveFile(agentId, file.id, approved)
        );

        if (error) {
            console.error("Failed to approve file", error);
        }

        mutateFiles((prev) =>
            prev?.map((f) => (f.id === file.id ? (updatedFile ?? file) : f))
        );
    };

    return {
        files: sortedFiles,
        reingestFile,
        approveFile,
        mutateFiles,
        startPollingFiles: startPolling,
        ...rest,
    };
}
