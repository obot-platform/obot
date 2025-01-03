import { useEffect, useMemo, useState } from "react";

import {
    KnowledgeFile,
    KnowledgeFileState,
    KnowledgeSource,
    KnowledgeSourceNamespace,
    KnowledgeSourceStatus,
} from "~/lib/model/knowledge";
import { KnowledgeSourceApiService } from "~/lib/service/api/knowledgeSourceApiService";
import { handlePromise } from "~/lib/service/async";

export function useKnowledgeSourceFiles(
    namespace: KnowledgeSourceNamespace,
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

    const [files, setFiles] = useState<KnowledgeFile[]>([]);

    useEffect(() => {
        const eventSource =
            KnowledgeSourceApiService.getKnowledgeSourceFilesEventSource(
                namespace,
                agentId,
                knowledgeSource.id
            );

        eventSource.onmessage = (event) => {
            const payload = JSON.parse(event.data);
            const { eventType, knowledgeFile } = payload;

            setFiles((prevFiles) => {
                let updatedFiles = [...prevFiles];
                switch (eventType) {
                    case "ADDED":
                    case "MODIFIED":
                        {
                            const existingIndex = updatedFiles.findIndex(
                                (file) => file.id === knowledgeFile.id
                            );
                            if (existingIndex !== -1) {
                                updatedFiles[existingIndex] = knowledgeFile;
                            } else {
                                updatedFiles.push(knowledgeFile);
                            }
                        }
                        break;
                    case "DELETED":
                        {
                            updatedFiles = updatedFiles.filter(
                                (file) => file.id !== knowledgeFile.id
                            );
                        }
                        break;
                    default:
                        break;
                }
                return updatedFiles;
            });
        };

        return () => {
            setFiles([]);
            eventSource.close();
        };
    }, [knowledgeSource.id, namespace, agentId]);

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
        const updatedFile =
            await KnowledgeSourceApiService.reingestFileFromSource(
                namespace,
                agentId,
                knowledgeSource.id,
                fileId
            );
        setFiles((prev) =>
            prev?.map((f) => (f.id === fileId ? updatedFile : f))
        );
    };

    const approveFile = async (file: KnowledgeFile, approved: boolean) => {
        const { error, data: updatedFile } = await handlePromise(
            KnowledgeSourceApiService.approveFile(
                namespace,
                agentId,
                file.id,
                approved
            )
        );

        if (error) {
            console.error("Failed to approve file", error);
        }

        setFiles((prev) =>
            prev?.map((f) => (f.id === file.id ? (updatedFile ?? file) : f))
        );
    };

    return {
        files: sortedFiles,
        reingestFile,
        approveFile,
        startPollingFiles: startPolling,
    };
}
