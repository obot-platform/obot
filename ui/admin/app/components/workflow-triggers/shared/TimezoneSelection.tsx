import { Label } from "~/components/ui/label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "~/components/ui/select";

type TimezoneSelectionProps = {
	label?: string;
	onChange: (timezone: string) => void;
	value?: string;
};

export function TimezoneSelection({
	label,
	onChange,
	value,
}: TimezoneSelectionProps) {
	const defaultTimezone = Intl.DateTimeFormat().resolvedOptions().timeZone;

	const getOffset = (timezone: string) => {
		const date = new Date();
		const offset = new Intl.DateTimeFormat("en-US", {
			timeZone: timezone,
			timeZoneName: "longOffset",
		})
			.format(date)
			.split(" ")
			.pop();
		return offset;
	};

	return (
		<fieldset className="flex flex-col gap-3">
			{label && <Label>{label}</Label>}
			<Select value={value ?? defaultTimezone} onValueChange={onChange}>
				<SelectTrigger className="flex-1">
					<SelectValue />
				</SelectTrigger>
				<SelectContent>
					{Intl.supportedValuesOf("timeZone").map((timezone) => {
						return (
							<SelectItem
								key={timezone}
								value={timezone}
								className="cursor-pointer"
							>
								{timezone} ({getOffset(timezone)})
							</SelectItem>
						);
					})}
				</SelectContent>
			</Select>
		</fieldset>
	);
}
