import { ClockIcon, PlusIcon, TrashIcon } from "lucide-react";

import { CronJob } from "~/lib/model/cronjobs";

import { Button } from "~/components/ui/button";
import { CardDescription } from "~/components/ui/card";
import { ScheduleSelection } from "~/components/workflow-triggers/shared/ScheduleSelection";
import { useCronjob } from "~/hooks/cronjob/useCronjob";

const defaultSchedule = {
    schedule: "0 * * * *", // default: on the hour
    description: "",
    input: "",
};

export function WorkflowSchedulePanel({ workflowId }: { workflowId: string }) {
    const { cronJobs, createCronJob, deleteCronJob, updateCronJob } =
        useCronjob(workflowId);

    const handleCronJobScheduleUpdate = (
        cronJob: CronJob,
        newSchedule: string
    ) => {
        const { id: cronJobId, ...rest } = cronJob;
        updateCronJob(cronJobId, {
            ...rest,
            schedule: newSchedule,
        });
    };

    return (
        <div className="p-4 m-4 flex flex-col gap-4">
            <div className="flex justify-between items-center gap-2">
                <h4 className="flex items-center gap-2">
                    <ClockIcon className="w-4 h-4" />
                    Schedule
                </h4>
            </div>

            <CardDescription>
                Set up a schedule to run the workflow on a regular basis.
            </CardDescription>

            <div className="flex flex-col gap-4 justify-center items-center">
                {cronJobs.map((cronJob) => (
                    <div key={cronJob.id} className="flex gap-2 w-full">
                        <ScheduleSelection
                            onChange={(newSchedule) =>
                                handleCronJobScheduleUpdate(
                                    cronJob,
                                    newSchedule
                                )
                            }
                            value={cronJob?.schedule ?? ""}
                        />
                        <Button
                            size="icon"
                            variant="ghost"
                            onClick={() => deleteCronJob(cronJob.id)}
                        >
                            <TrashIcon className="w-4 h-4" />
                        </Button>
                    </div>
                ))}
            </div>

            <Button
                variant="ghost"
                className="self-end"
                startContent={<PlusIcon />}
                type="button"
                loading={createCronJob.isLoading}
                onClick={() =>
                    createCronJob.execute({
                        ...defaultSchedule,
                        workflow: workflowId,
                    })
                }
            >
                Add Schedule
            </Button>
        </div>
    );
}
