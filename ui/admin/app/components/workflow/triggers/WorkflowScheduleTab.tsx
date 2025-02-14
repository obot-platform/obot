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
	taskSchedule: {
		interval: "hourly" as const,
		minute: 0,
		hour: 0,
		day: 0,
		weekday: 0,
		timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
	},
};

export function WorkflowScheduleTab({ workflowId }: { workflowId: string }) {
	const { cronJobs, createCronJob, deleteCronJob, updateCronJob } =
		useCronjob(workflowId);

	const handleCronJobScheduleUpdate = (
		cronJob: CronJob,
		cronString: string
	) => {
		const { id: cronJobId, ...rest } = cronJob;
		updateCronJob(cronJobId, {
			...rest,
			schedule: cronString,
			taskSchedule: cronToTaskSchedule(cronString, cronJob.timezone),
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
							value={
								(cronJob?.taskSchedule
									? taskScheduleToCron(cronJob.taskSchedule)
									: cronJob?.schedule) ?? ""
							}
							timezone={cronJob?.timezone ?? ""}
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
						timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
					})
				}
			>
				Add Schedule
			</Button>
		</div>
	);

	function taskScheduleToCron(
		schedule: CronJob["taskSchedule"]
	): string | null {
		if (!schedule) return null;

		switch (schedule.interval) {
			case "hourly":
				if (schedule.minute === 0) {
					return "0 * * * *"; // On the hour
				}
				return `*/${schedule.minute} * * * *`; // Every X minutes

			case "daily":
				return `${schedule.minute} ${schedule.hour} * * *`;

			case "weekly":
				return `${schedule.minute} ${schedule.hour} * * ${schedule.weekday}`;

			case "monthly": {
				// -1 represents last day & days in taskSchedule start from 0 index
				const day = schedule.day === -1 ? "L" : schedule.day + 1;
				return `${schedule.minute} ${schedule.hour} ${day} * *`;
			}
			default:
				return "0 * * * *"; // Default to hourly
		}
	}

	function cronToTaskSchedule(
		cronString: string,
		timezone: string
	): CronJob["taskSchedule"] | undefined {
		// Match groups: (minute) (hour) (day) (month) (weekday)
		const matches = cronString.match(/^(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(\S+)$/);
		if (!matches) return;

		const [_, minute, hour, day, _month, weekday] = matches;

		// Hourly patterns
		if (minute === "0" && hour === "*") {
			return {
				interval: "hourly",
				minute: 0,
				hour: 0,
				day: 0,
				weekday: 0,
				timezone,
			};
		}
		const minuteInterval = minute.match(/^\*\/(\d+)$/);
		if (minuteInterval && hour === "*") {
			return {
				interval: "hourly",
				minute: parseInt(minuteInterval[1]),
				hour: 0,
				day: 0,
				weekday: 0,
				timezone,
			};
		}

		// Daily pattern: 0 6 * * *
		if (
			minute === "0" &&
			/^\d+$/.test(hour) &&
			day === "*" &&
			weekday === "*"
		) {
			return {
				interval: "daily",
				minute: 0,
				hour: parseInt(hour),
				day: 0,
				weekday: 0,
				timezone,
			};
		}

		// Weekly pattern: 0 0 * * 3
		if (minute === "0" && hour === "0" && day === "*" && /^\d$/.test(weekday)) {
			return {
				interval: "weekly",
				minute: 0,
				hour: 0,
				day: 0,
				weekday: parseInt(weekday),
				timezone,
			};
		}

		// Monthly patterns: "0 0 15 * *" or "0 0 L * *"
		if (
			minute === "0" &&
			hour === "0" &&
			(day === "L" || /^\d+$/.test(day)) &&
			weekday === "*"
		) {
			return {
				interval: "monthly",
				minute: 0,
				hour: 0,
				day: day === "L" ? -1 : parseInt(day) - 1, // day starts from 0 index
				weekday: 0,
				timezone,
			};
		}
	}
}
