import { Label } from "~/components/ui/label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "~/components/ui/select";
import { TimezoneSelection } from "~/components/workflow-triggers/shared/TimezoneSelection";

type ScheduleSelectionProps = {
	disabled?: boolean;
	label?: string;
	onChange: (schedule: string) => void;
	value: string;
	timezone: string;
};

export function ScheduleSelection({
	disabled,
	label,
	onChange,
	value,
	timezone,
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
		<fieldset className="flex w-full flex-col gap-3">
			{label && <Label>{label}</Label>}
			<div className="flex w-full gap-4">
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
							<SelectItem key={timeOption.value} value={timeOption.value}>
								{timeOption.label}
							</SelectItem>
						))}
					</SelectContent>
				</Select>
				<TimezoneSelection timezone={timezone} />
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
		monthly: /^0 \d+ (\d+|L) \* \*$/, // "0 0 15 * *" or "0 0 L * *"
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
				{ label: "15 Minutes Past", value: "*/15 * * * *" },
				{ label: "30 Minutes Past", value: "*/30 * * * *" },
				{ label: "45 Minutes Past", value: "*/45 * * * *" },
			];
		case "daily":
			return [
				{ label: "Midnight", value: "0 0 * * *" },
				{ label: "3 AM", value: "0 3 * * *" },
				{ label: "6 AM", value: "0 6 * * *" },
				{ label: "9 AM", value: "0 9 * * *" },
				{ label: "Noon", value: "0 12 * * *" },
				{ label: "3 PM", value: "0 15 * * *" },
				{ label: "6 PM", value: "0 18 * * *" },
				{ label: "9 PM", value: "0 21 * * *" },
			];
		case "weekly":
			return [
				{ label: "Sunday", value: "0 0 * * 0" },
				{ label: "Monday", value: "0 0 * * 1" },
				{ label: "Tuesday", value: "0 0 * * 2" },
				{ label: "Wednesday", value: "0 0 * * 3" },
				{ label: "Thursday", value: "0 0 * * 4" },
				{ label: "Friday", value: "0 0 * * 5" },
				{ label: "Saturday", value: "0 0 * * 6" },
			];
		case "monthly":
			return [
				{ label: "1st", value: "0 0 1 * *" },
				{ label: "2nd", value: "0 0 2 * *" },
				{ label: "3rd", value: "0 0 3 * *" },
				{ label: "5th", value: "0 0 5 * *" },
				{ label: "15th", value: "0 0 15 * *" },
				{ label: "20th", value: "0 0 20 * *" },
				{ label: "25th", value: "0 0 25 * *" },
				{
					label: "Last Day",
					value: "0 0 L * *",
				},
			];
		default:
			return [];
	}
}
