import React, { useEffect, useState } from "react";

import { KnowledgeSource } from "~/lib/model/knowledge";
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
import { useAsync } from "~/hooks/useAsync";

type RemoteSourceSettingModalProps = {
    agentId: string;
    isOpen: boolean;
    onOpenChange: (open: boolean) => void;
    knowledgeSource: KnowledgeSource | undefined;
};

const RemoteSourceSettingModal: React.FC<RemoteSourceSettingModalProps> = ({
    agentId,
    isOpen,
    onOpenChange,
    knowledgeSource,
}) => {
    const [autoApprove, setAutoApprove] = useState(false);

    useEffect(() => {
        setAutoApprove(knowledgeSource?.autoApprove || false);
    }, [knowledgeSource]);

    const [syncSchedule, setSyncSchedule] = useState("");

    useEffect(() => {
        setSyncSchedule(knowledgeSource?.syncSchedule || "");
    }, [knowledgeSource]);

    const updateRemoteKnowledgeSource = async () => {
        await KnowledgeService.updateKnowledgeSource(
            agentId,
            knowledgeSource!.id,
            {
                ...knowledgeSource,
                syncSchedule,
                autoApprove,
            }
        );
        onOpenChange(false);
    };

    const handleSave = useAsync(updateRemoteKnowledgeSource);

    return (
        <Dialog open={isOpen} onOpenChange={onOpenChange}>
            <DialogContent aria-describedby={undefined}>
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
                            onClick={() => setAutoApprove((prev) => !prev)}
                        />
                        Include new pages
                    </div>
                    <p className="text-sm text-gray-500 mt-4">
                        If enabled, new pages will be added to the knowledge
                        base automatically.
                    </p>
                </div>
                <DialogFooter>
                    <Button onClick={handleSave.execute}>Save</Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
};

export default RemoteSourceSettingModal;
