import { useCallback, useRef, useState } from "react";
import { $path } from "safe-routes";
import useSWR from "swr";

import { Agent, KNOWLEDGE_TOOL } from "~/lib/model/agents";
import {
    KnowledgeSourceType,
    getKnowledgeSourceType,
} from "~/lib/model/knowledge";
import { ModelAlias } from "~/lib/model/models";
import { DefaultModelAliasApiService } from "~/lib/service/api/defaultModelAliasApiService";

import { TypographyP } from "~/components/Typography";
import { ErrorDialog } from "~/components/composed/ErrorDialog";
import { WarningAlert } from "~/components/composed/WarningAlert";
import { AddKnowledgeButton } from "~/components/knowledge/AddKnowledgeButton";
import { AddSourceModal } from "~/components/knowledge/AddSourceModal";
import { KnowledgeFileItem } from "~/components/knowledge/KnowledgeFileItem";
import { KnowledgeSourceDetail } from "~/components/knowledge/KnowledgeSourceDetail";
import { KnowledgeSourceItem } from "~/components/knowledge/KnowledgeSourceItem";
import { Input } from "~/components/ui/input";
import { Label } from "~/components/ui/label";
import { Link } from "~/components/ui/link";
import { AutosizeTextarea } from "~/components/ui/textarea";
import { useKnowledgeFiles } from "~/hooks/knowledge/useKnowledgeFiles";
import { useKnowledgeSources } from "~/hooks/knowledge/useKnowledgeSources";
import { useMultiAsync } from "~/hooks/useMultiAsync";

type AgentKnowledgePanelProps = {
    agentId: string;
    agent: Agent;
    updateAgent: (updatedAgent: Agent) => void;
    addTool: (tool: string) => void;
};

export default function AgentKnowledgePanel({
    agentId,
    agent,
    updateAgent,
    addTool,
}: AgentKnowledgePanelProps) {
    const fileInputRef = useRef<HTMLInputElement>(null);
    const [isAddSourceModalOpen, setIsAddSourceModalOpen] = useState(false);
    const [sourceType, setSourceType] = useState<KnowledgeSourceType>(
        KnowledgeSourceType.Website
    );
    const [selectedKnowledgeSourceId, setSelectedKnowledgeSourceId] = useState<
        string | undefined
    >(undefined);
    const [isEditKnowledgeSourceModalOpen, setIsEditKnowledgeSourceModalOpen] =
        useState(false);
    const [errorDialogError, setErrorDialogError] = useState("");

    const { data: defaultAliases } = useSWR(
        DefaultModelAliasApiService.getAliases.key(),
        DefaultModelAliasApiService.getAliases
    );

    const { localFiles, addKnowledgeFile, deleteKnowledgeFile, reingestFile } =
        useKnowledgeFiles("agents", agentId);

    const {
        knowledgeSources,
        syncKnowledgeSource,
        deleteKnowledgeSource,
        updateKnowledgeSource,
        addWebsite,
        addOneDrive,
        addNotion,
    } = useKnowledgeSources("agents", agentId);

    const selectedKnowledgeSource = knowledgeSources.find(
        (source) => source.id === selectedKnowledgeSourceId
    );

    const handleAddKnowledge = useCallback(
        async (_index: number, file: File) => {
            const addedFile = await addKnowledgeFile(file);
            return addedFile;
        },
        [addKnowledgeFile]
    );

    const uploadKnowledge = useMultiAsync(handleAddKnowledge);

    const startUpload = (files: FileList) => {
        if (!files.length) return;

        uploadKnowledge.execute(
            Array.from(files).map((file) => [file] as const)
        );

        addTool(KNOWLEDGE_TOOL);

        if (fileInputRef.current) fileInputRef.current.value = "";
    };

    const hasDefaultTextEmbedding = defaultAliases?.some(
        (alias) => alias.alias === ModelAlias.TextEmbedding && !!alias.model
    );

    const handleSave = (knowledgeSourceId: string): void => {
        addTool(KNOWLEDGE_TOOL);
        setSelectedKnowledgeSourceId(knowledgeSourceId);
        setIsEditKnowledgeSourceModalOpen(true);
    };

    const handleAddWebsite = async (website: string) => {
        const res = await addWebsite(website);
        handleSave(res.id);
    };

    const handleAddOneDrive = async (link: string) => {
        const res = await addOneDrive(link);
        handleSave(res.id);
    };

    const handleAddNotion = async () => {
        const res = await addNotion();
        handleSave(res.id);
    };

    return (
        <div className="flex flex-col gap-4 justify-center items-center">
            {!hasDefaultTextEmbedding && (
                <WarningAlert
                    title="Default Text Embedding Model Required!"
                    description={
                        <TypographyP>
                            In order to process the knowledge base for your
                            agent, you&apos;ll need to set up a default text
                            embedding model. Click{" "}
                            <Link to={$path("/model-providers")}>here</Link> to
                            update your model provider and/or default models.
                        </TypographyP>
                    }
                />
            )}
            <div className="grid w-full gap-2">
                <Label htmlFor="message">Knowledge Description</Label>
                <AutosizeTextarea
                    disabled={!hasDefaultTextEmbedding}
                    defaultValue={agent.knowledgeDescription}
                    maxHeight={200}
                    placeholder="Provide a brief description of the information contained in this knowledge base. Example: A collection of documents about the human resources policies and procedures for Acme Corporation."
                    id="message"
                    onChange={(e) =>
                        updateAgent({
                            ...agent,
                            knowledgeDescription: e.target.value,
                        })
                    }
                    className="max-h-[400px]"
                />
            </div>

            <div className="flex flex-col gap-2 w-full">
                {localFiles.map((file) => (
                    <KnowledgeFileItem
                        key={file.fileName}
                        file={file}
                        onDelete={deleteKnowledgeFile}
                        onReingest={(file) => reingestFile(file.id!)}
                        onViewError={setErrorDialogError}
                    />
                ))}

                {knowledgeSources.map((source) => (
                    <KnowledgeSourceItem
                        key={source.id}
                        source={source}
                        onSync={syncKnowledgeSource}
                        onEdit={(id) => {
                            setSelectedKnowledgeSourceId(id);
                            setIsEditKnowledgeSourceModalOpen(true);
                        }}
                        onDelete={deleteKnowledgeSource}
                    />
                ))}
                <AddKnowledgeButton
                    disabled={!hasDefaultTextEmbedding}
                    onUploadFiles={() => fileInputRef.current?.click()}
                    onAddSource={(type) => {
                        if (type === KnowledgeSourceType.Notion) {
                            handleAddNotion();
                        } else {
                            setSourceType(type);
                            setIsAddSourceModalOpen(true);
                        }
                    }}
                    hasExistingNotion={knowledgeSources.some(
                        (source) =>
                            getKnowledgeSourceType(source) ===
                            KnowledgeSourceType.Notion
                    )}
                />
            </div>

            <AddSourceModal
                isOpen={isAddSourceModalOpen}
                sourceType={sourceType}
                onOpenChange={setIsAddSourceModalOpen}
                startPolling={() => {}}
                addTool={addTool}
                onAddWebsite={handleAddWebsite}
                onAddOneDrive={handleAddOneDrive}
            />
            <ErrorDialog
                error={errorDialogError}
                isOpen={errorDialogError !== ""}
                onClose={() => setErrorDialogError("")}
            />
            {selectedKnowledgeSourceId && selectedKnowledgeSource && (
                <KnowledgeSourceDetail
                    agentId={agentId}
                    knowledgeSource={selectedKnowledgeSource}
                    isOpen={isEditKnowledgeSourceModalOpen}
                    onOpenChange={setIsEditKnowledgeSourceModalOpen}
                    onSyncNow={() =>
                        syncKnowledgeSource(selectedKnowledgeSourceId)
                    }
                    onDelete={() =>
                        deleteKnowledgeSource(selectedKnowledgeSourceId)
                    }
                    onUpdate={(source) =>
                        updateKnowledgeSource(selectedKnowledgeSourceId, source)
                    }
                />
            )}
            <Input
                ref={fileInputRef}
                type="file"
                className="hidden"
                multiple
                onChange={(e) => {
                    if (!e.target.files) return;
                    startUpload(e.target.files);
                }}
            />
        </div>
    );
}
