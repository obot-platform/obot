import { Label } from "~/components/ui/label";
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from "~/components/ui/select";

type ScheduleSelectionProps = {
    disabled?: boolean;
    label?: string;
    onChange: (schedule: string) => void;
    value: string;
};

export function ScheduleSelection({
    disabled,
    label,
    onChange,
    value,
}: ScheduleSelectionProps) {
    const cronFrequency = getCronFrequency(value ?? "");

    const timeOptions = cronFrequency
        ? getTimeOptionsForInterval(cronFrequency)
        : [];

    const handleOnChange = (value: string) => {
        if (!value) return;
        onChange(value);
    };

    const handleFrequencyChange = (
        value: "hourly" | "daily" | "weekly" | "monthly"
    ) => {
        const newCronSchedule = getFrequencyCron(value);
        handleOnChange(newCronSchedule);
    };

    return (
        <fieldset className="flex flex-col gap-3 w-full">
            {label && <Label>{label}</Label>}
            <div className="flex gap-4 w-full">
                <Select
                    disabled={disabled}
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
                    disabled={disabled}
                    value={value || timeOptions[0]?.value}
                    onValueChange={handleOnChange}
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
        </fieldset>
    );
}

function getCronFrequency(
    cronString: string
): "hourly" | "daily" | "weekly" | "monthly" | null {
    const patterns = {
        hourly: /^(0|\*\/\d+) \* \* \* \*$/, // ex. "0 * * * *" or "*/15 * * * *"
        daily: /^0 \d+ \* \* \*$/, // ex. "0 6 * * *"
        weekly: /^0 \d+ \* \* \d$/, // ex. "0 0 * * 3"
        monthly: /^0 \d+ \d+ \* \*$/, // "0 0 15 * *"
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
