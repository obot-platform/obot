import React, { useEffect, useState } from "react";

import { RemoteKnowledgeSource } from "~/lib/model/knowledge";
import { KnowledgeService } from "~/lib/service/api/knowledgeService";

import { Button } from "~/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogFooter,
    DialogHeader,
    DialogTitle,
} from "~/components/ui/dialog";
import { Input } from "~/components/ui/input";
import { Switch } from "~/components/ui/switch";

type RemoteSourceSettingModalProps = {
    agentId: string;
    isOpen: boolean;
    onOpenChange: (open: boolean) => void;
    remoteKnowledgeSource: RemoteKnowledgeSource;
};

const RemoteSourceSettingModal: React.FC<RemoteSourceSettingModalProps> = ({
    agentId,
    isOpen,
    onOpenChange,
    remoteKnowledgeSource,
}) => {
    const [autoApprove, setAutoApprove] = useState(
        remoteKnowledgeSource.autoApprove || false
    );

    const [syncSchedule, setSyncSchedule] = useState(
        remoteKnowledgeSource.syncSchedule || ""
    );

    useEffect(() => {
        setSyncSchedule(remoteKnowledgeSource.syncSchedule || "");
    }, [remoteKnowledgeSource]);

    const handleSave = async () => {
        try {
            await KnowledgeService.updateRemoteKnowledgeSource(
                agentId,
                remoteKnowledgeSource.id,
                {
                    ...remoteKnowledgeSource,
                    syncSchedule,
                    autoApprove,
                }
            );
            onOpenChange(false);
        } catch (error) {
            console.error("Failed to update sync schedule:", error);
        }
    };

    return (
        <Dialog open={isOpen} onOpenChange={onOpenChange}>
            <DialogContent>
                <DialogHeader>
                    <DialogTitle>Update Source Settings</DialogTitle>
                </DialogHeader>
                <div className="mb-2">
                    <label
                        htmlFor="syncSchedule"
                        className="block text-sm font-medium text-gray-700"
                    >
                        Sync Schedule (Cron Syntax)
                    </label>
                    <Input
                        type="text"
                        value={syncSchedule}
                        onChange={(e) => setSyncSchedule(e.target.value)}
                        placeholder="Enter cron syntax"
                        className="w-full mt-2 mb-4"
                    />
                    <div>
                        <p className="text-sm text-gray-500">
                            You can use a cron syntax to define the sync
                            schedule. For example, &quot;0 0 * * *&quot; means
                            every day at midnight.
                        </p>
                    </div>
                </div>
                <hr className="my-4" />
                <div className="mb-4">
                    <div className="flex items-center">
                        <Switch
                            id="autoApprove"
                            className="mr-2"
                            checked={autoApprove}
                            onChange={() => setAutoApprove(!autoApprove)}
                        />
                        <label
                            htmlFor="autoApprove"
                            className="text-sm text-gray-600 dark:text-gray-400 mr-2"
                        >
                            Include new pages
                        </label>
                    </div>
                    <p className="text-sm text-gray-500 mt-4">
                        If enabled, new pages will be added to the knowledge
                        base automatically.
                    </p>
                </div>
                <DialogFooter>
                    <Button onClick={handleSave}>Save</Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
};

export default RemoteSourceSettingModal;
