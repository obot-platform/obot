import { ClockIcon } from "lucide-react";

import { TypographyH4 } from "~/components/Typography";
import { CardDescription } from "~/components/ui/card";
import { Label } from "~/components/ui/label";
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from "~/components/ui/select";
import { Switch } from "~/components/ui/switch";
import { useCronjob } from "~/hooks/cronjob/useCronjob";

export function WorkflowSchedulePanel({ workflowId }: { workflowId: string }) {
    const { cronJob, createCronJob, deleteCronJob, updateCronJob } =
        useCronjob(workflowId);
    const cronFrequency = getCronFrequency(cronJob?.schedule ?? "");
    const timeOptions = cronFrequency
        ? getTimeOptionsForInterval(cronFrequency)
        : [];

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

    const handleFrequencyChange = (
        value: "hourly" | "daily" | "weekly" | "monthly"
    ) => {
        const newCronSchedule = getFrequencyCron(value);
        handleCronJobScheduleUpdate(newCronSchedule);
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
                <Select
                    disabled={!hasCronJob}
                    value={cronFrequency ?? undefined}
                    onValueChange={handleFrequencyChange}
                >
                    <SelectTrigger>
                        <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                        <SelectItem value="hourly">Hourly</SelectItem>
                        <SelectItem value="daily">Daily</SelectItem>
                        <SelectItem value="weekly">Weekly</SelectItem>
                        <SelectItem value="monthly">Monthly</SelectItem>
                    </SelectContent>
                </Select>
                <Select
                    disabled={!hasCronJob}
                    value={cronJob?.schedule || timeOptions[0]?.value}
                    onValueChange={handleCronJobScheduleUpdate}
                >
                    <SelectTrigger>
                        <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                        {timeOptions.map((timeOption) => (
                            <SelectItem
                                key={timeOption.value}
                                value={timeOption.value}
                            >
                                {timeOption.label}
                            </SelectItem>
                        ))}
                    </SelectContent>
                </Select>
            </div>
        </div>
    );
}

function getCronFrequency(
    cronString: string
): "hourly" | "daily" | "weekly" | "monthly" | null {
    const patterns = {
        hourly: /^(0|\*\/\d+)\s+\*\s+\*\s+\*\s+\*$/, // ex. "0 * * * *" or "*/15 * * * *"
        daily: /^0\s+\d+\s+\*\s+\*\s+\*$/, // ex. "0 6 * * *"
        weekly: /^0\s+\d+\s+\*\s+\*\s+\d$/, // ex. "0 0 * * 3"
        monthly: /^0\s+\d+\s+\d+\s+\*\s+\*$/, // "0 0 15 * *"
    };

    for (const [frequency, pattern] of Object.entries(patterns)) {
        if (pattern.test(cronString)) {
            return frequency as "hourly" | "daily" | "weekly" | "monthly";
        }
    }

    return null;
}

function getFrequencyCron(
    frequency: "hourly" | "daily" | "weekly" | "monthly"
): string {
    switch (frequency) {
        case "hourly":
            return "0 * * * *"; // At minute 0 of every hour
        case "daily":
            return "0 0 * * *"; // At midnight every day
        case "weekly":
            return "0 0 * * 0"; // At midnight on Sunday
        case "monthly":
            return "0 0 1 * *"; // At midnight on the 1st of every month
        default:
            return "0 * * * *"; // Default to hourly if invalid input
    }
}

function getTimeOptionsForInterval(interval: string) {
    switch (interval) {
        case "hourly":
            return [
                { label: "On The Hour", value: "0 * * * *" },
                { label: "Every 15 Minutes", value: "*/15 * * * *" },
                { label: "Every 30 Minutes", value: "*/30 * * * *" },
            ];
        case "daily":
            return [
                { label: "At Midnight", value: "0 0 * * *" },
                { label: "At 6:00 AM", value: "0 6 * * *" },
                { label: "At Noon", value: "0 12 * * *" },
                { label: "At 6:00 PM", value: "0 18 * * *" },
            ];
        case "weekly":
            return [
                { label: "Sunday at Midnight", value: "0 0 * * 0" },
                { label: "Monday at Midnight", value: "0 0 * * 1" },
                { label: "Wednesday at Midnight", value: "0 0 * * 3" },
                { label: "Friday at Midnight", value: "0 0 * * 5" },
            ];
        case "monthly":
            return [
                { label: "1st at Midnight", value: "0 0 1 * *" },
                { label: "15th at Midnight", value: "0 0 15 * *" },
                {
                    label: "Last Day at Midnight",
                    value: "0 0 L * *",
                },
            ];
        default:
            return [];
    }
}
