import { useMemo, useState } from "react";
import useSWR from "swr";

import {
    KnowledgeNamespace,
    KnowledgeSource,
    KnowledgeSourceInput,
    KnowledgeSourceStatus,
} from "~/lib/model/knowledge";
import { KnowledgeSourceApiService } from "~/lib/service/api/knowledgeSourceApiService";

export function useKnowledgeSources(
    namespace: KnowledgeNamespace,
    agentId: string
) {
    const [blockPollingSources, setBlockPollingSources] = useState(false);
    const startPolling = () => setBlockPollingSources(false);

    const {
        data: sources,
        mutate: mutateSources,
        ...rest
    } = useSWR(
        KnowledgeSourceApiService.getKnowledgeSources.key(namespace, agentId),
        ({ namespace, agentId }) =>
            KnowledgeSourceApiService.getKnowledgeSources(namespace, agentId),
        {
            revalidateOnFocus: false,
            refreshInterval: blockPollingSources ? undefined : 5000,
        }
    );

    const knowledgeSources = useMemo(() => {
        return sources?.filter((source) => !source.deleted) || [];
    }, [sources]);

    const shouldBlockPolling =
        knowledgeSources.length === 0 ||
        knowledgeSources.every(
            (source) =>
                source.state === KnowledgeSourceStatus.Synced ||
                source.state === KnowledgeSourceStatus.Error
        );

    if (shouldBlockPolling !== blockPollingSources) {
        setBlockPollingSources(shouldBlockPolling);
    }

    const syncKnowledgeSource = async (sourceId: string) => {
        const syncedSource =
            await KnowledgeSourceApiService.resyncKnowledgeSource(
                namespace,
                agentId,
                sourceId
            );
        mutateSources((prev) =>
            prev?.map((source) =>
                source.id === syncedSource.id ? syncedSource : source
            )
        );
        return syncedSource;
    };

    const deleteKnowledgeSource = async (sourceId: string) => {
        await KnowledgeSourceApiService.deleteKnowledgeSource(
            namespace,
            agentId,
            sourceId
        );
        mutateSources(
            (prev) => prev?.filter((source) => source.id !== sourceId),
            false
        );
    };

    const updateKnowledgeSource = async (
        sourceId: string,
        updates: Partial<KnowledgeSource>
    ) => {
        const source = knowledgeSources.find((s) => s.id === sourceId);
        if (!source) throw new Error("Source not found");

        const updatedSource =
            await KnowledgeSourceApiService.updateKnowledgeSource(
                namespace,
                agentId,
                sourceId,
                { ...source, ...updates }
            );
        mutateSources((prev) =>
            prev?.map((s) => (s.id === updatedSource.id ? updatedSource : s))
        );
        return updatedSource;
    };

    const createKnowledgeSource = async (config: KnowledgeSourceInput) => {
        const newSource = await KnowledgeSourceApiService.createKnowledgeSource(
            namespace,
            agentId,
            config
        );
        mutateSources();
        startPolling();
        return newSource;
    };

    const addWebsite = async (website: string) => {
        const trimmedWebsite = website.trim();
        const formattedWebsite =
            trimmedWebsite.startsWith("http://") ||
            trimmedWebsite.startsWith("https://")
                ? trimmedWebsite
                : `https://${trimmedWebsite}`;

        return await createKnowledgeSource({
            websiteCrawlingConfig: { urls: [formattedWebsite] },
        });
    };

    const addOneDrive = async (link: string) => {
        return await createKnowledgeSource({
            onedriveConfig: { sharedLinks: [link.trim()] },
        });
    };

    const addNotion = async () => {
        return await createKnowledgeSource({ notionConfig: {} });
    };

    return {
        knowledgeSources,
        syncKnowledgeSource,
        deleteKnowledgeSource,
        updateKnowledgeSource,
        createKnowledgeSource,
        mutateSources,
        addWebsite,
        addOneDrive,
        addNotion,
        ...rest,
    };
}
