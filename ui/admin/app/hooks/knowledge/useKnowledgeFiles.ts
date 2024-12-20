import { useMemo, useState } from "react";
import useSWR from "swr";

import {
    KnowledgeFile,
    KnowledgeFileNamespace,
    KnowledgeFileState,
} from "~/lib/model/knowledge";
import { KnowledgeFileService } from "~/lib/service/api/knowledgeFileApiService";

export function useKnowledgeFiles(
    namespace: KnowledgeFileNamespace,
    agentId: string
) {
    const [blockPollingLocalFiles, setBlockPollingLocalFiles] = useState(false);

    const {
        data: files,
        mutate: mutateFiles,
        ...rest
    } = useSWR(
        KnowledgeFileService.getKnowledgeFiles.key(namespace, agentId),
        ({ namespace, agentId }) =>
            KnowledgeFileService.getKnowledgeFiles(namespace, agentId),
        {
            revalidateOnFocus: false,
            refreshInterval: blockPollingLocalFiles ? undefined : 5000,
        }
    );

    const localFiles = useMemo(() => {
        return (
            files
                ?.sort((a, b) => a.fileName.localeCompare(b.fileName))
                .map((item) => ({ ...item }) as KnowledgeFile)
                .filter((item) => !item.deleted) || []
        );
    }, [files]);

    const shouldBlock = useMemo(
        () =>
            localFiles.every(
                (file) =>
                    file.state === KnowledgeFileState.Ingested ||
                    file.state === KnowledgeFileState.Error
            ),
        [localFiles]
    );

    if (shouldBlock !== blockPollingLocalFiles) {
        setBlockPollingLocalFiles(shouldBlock);
    }

    const addKnowledgeFile = async (file: File) => {
        const addedFile = await KnowledgeFileService.addKnowledgeFiles(
            namespace,
            agentId,
            file
        );
        mutateFiles((prev) => (prev ? [...prev, addedFile] : [addedFile]));
        return addedFile;
    };

    const deleteKnowledgeFile = async (file: KnowledgeFile) => {
        await KnowledgeFileService.deleteKnowledgeFile(
            namespace,
            agentId,
            file.fileName
        );
        mutateFiles((prev) => prev?.filter((f) => f.id !== file.id));
    };

    const reingestFile = async (fileId: string) => {
        const reingestedFile = await KnowledgeFileService.reingestFile(
            namespace,
            agentId,
            fileId
        );
        mutateFiles((prev) =>
            prev?.map((f) => (f.id === reingestedFile.id ? reingestedFile : f))
        );
        return reingestedFile;
    };

    return {
        localFiles,
        addKnowledgeFile,
        deleteKnowledgeFile,
        reingestFile,
        mutateFiles,
        ...rest,
    };
}
