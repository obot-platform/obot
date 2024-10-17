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

type RemoteSourceSettingModalProps = {
    agentId: string;
    isOpen: boolean;
    onOpenChange: (open: boolean) => void;
    remoteKnowledgeSource: RemoteKnowledgeSource | null;
};

const RemoteSourceSettingModal: React.FC<RemoteSourceSettingModalProps> = ({
    agentId,
    isOpen,
    onOpenChange,
    remoteKnowledgeSource,
}) => {
    if (!remoteKnowledgeSource) return null;

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
                    <DialogTitle>Update Sync Schedule</DialogTitle>
                </DialogHeader>
                <div className="mb-4">
                    <label className="block text-sm font-medium text-gray-700">
                        Sync Schedule (Cron Syntax)
                    </label>
                    <Input
                        type="text"
                        value={syncSchedule}
                        onChange={(e) => setSyncSchedule(e.target.value)}
                        placeholder="Enter cron syntax"
                        className="w-full mt-2"
                    />
                </div>
                <div className="mb-4">
                    <p className="text-sm text-gray-500">
                        You can use a cron syntax to define the sync schedule.
                        For example, "0 0 * * *" means every day at midnight.
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
