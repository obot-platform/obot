import { ClockIcon } from "lucide-react";

import { TypographyH4 } from "~/components/Typography";
import { CardDescription } from "~/components/ui/card";
import { Label } from "~/components/ui/label";
import { Switch } from "~/components/ui/switch";
import { ScheduleSelection } from "~/components/workflow-triggers/shared/ScheduleSelection";
import { useCronjob } from "~/hooks/cronjob/useCronjob";

export function WorkflowSchedulePanel({ workflowId }: { workflowId: string }) {
    const { cronJob, createCronJob, deleteCronJob, updateCronJob } =
        useCronjob(workflowId);

    const hasCronJob = !!cronJob;

    const handleCheckedChange = (checked: boolean) => {
        if (checked) {
            createCronJob.execute({
                workflow: workflowId,
                schedule: "0 * * * *", // default: on the hour
                description: "",
                input: "",
            });
        } else {
            if (cronJob) {
                deleteCronJob.execute(cronJob.id);
            }
        }
    };

    const handleCronJobScheduleUpdate = (newSchedule: string) => {
        if (!cronJob) return;
        updateCronJob.execute(cronJob.id, {
            ...cronJob,
            schedule: newSchedule,
        });
    };

    return (
        <div className="p-4 m-4 flex flex-col gap-4">
            <div className="flex justify-between items-center gap-2">
                <TypographyH4 className="flex items-center gap-2">
                    <ClockIcon className="w-4 h-4" />
                    Schedule
                </TypographyH4>
                <div className="flex items-center space-x-2">
                    <Label htmlFor="schedule-switch" className="hidden">
                        Enable Schedule
                    </Label>
                    <Switch
                        id="schedule-switch"
                        checked={hasCronJob}
                        onCheckedChange={handleCheckedChange}
                    />
                </div>
            </div>

            <CardDescription>
                Set up a schedule to run the workflow on a regular basis.
            </CardDescription>

            <div className="flex gap-4 justify-center items-center">
                <ScheduleSelection
                    disabled={!hasCronJob}
                    onChange={handleCronJobScheduleUpdate}
                    value={cronJob?.schedule ?? ""}
                />
            </div>
        </div>
    );
}
