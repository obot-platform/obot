import { PlusIcon, TrashIcon } from "lucide-react";

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

export function WorkflowScheduleTab({ workflowId }: { workflowId: string }) {
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
		<div className="flex flex-col gap-4">
			<CardDescription>
				Set up a schedule to run the workflow on a regular basis.
			</CardDescription>

			<div className="flex flex-col items-center justify-center gap-4">
				{cronJobs.map((cronJob) => (
					<div key={cronJob.id} className="flex w-full gap-2">
						<ScheduleSelection
							onChange={(newSchedule) =>
								handleCronJobScheduleUpdate(cronJob, newSchedule)
							}
							value={cronJob?.schedule ?? ""}
						/>
						<Button
							size="icon"
							variant="ghost"
							onClick={() => deleteCronJob(cronJob.id)}
						>
							<TrashIcon className="h-4 w-4" />
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
