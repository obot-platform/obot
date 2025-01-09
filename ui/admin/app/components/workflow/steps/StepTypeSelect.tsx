import { ArrowRight, RotateCwIcon, SplitIcon } from "lucide-react";

import { StepType } from "~/lib/model/workflows";

import { ClickableDiv } from "~/components/ui/clickable-div";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "~/components/ui/select";

type StepSelectProps = {
	value: StepType;
	onChange: (type: StepType) => void;
};

export function StepTypeSelect({ value, onChange }: StepSelectProps) {
	const options = Object.values(StepType).map((type) => ({
		id: type,
		name: StepLabelMap[type],
	}));

	return (
		<ClickableDiv onClick={(e) => e.stopPropagation()}>
			<Select value={value} onValueChange={onChange}>
				<SelectTrigger className="w-28 bg-background">
					<SelectValue />
				</SelectTrigger>

				<SelectContent>
					{options.map((option) => (
						<SelectItem key={option.id} value={option.id}>
							<div className="flex items-center gap-2">
								<StepTypeIcon type={option.id} />
								{option.name}
							</div>
						</SelectItem>
					))}
				</SelectContent>
			</Select>
		</ClickableDiv>
	);
}

const StepLabelMap: Record<StepType, string> = {
	[StepType.Command]: "Step",
	[StepType.If]: "If",
	[StepType.While]: "While",
	// [StepType.Template]: "Template",
};

const IconMap: Record<StepType, React.ElementType> = {
	[StepType.Command]: ArrowRight,
	[StepType.If]: SplitIcon,
	[StepType.While]: RotateCwIcon,
	// [StepType.Template]: PuzzleIcon,
};

function StepTypeIcon({ type }: { type: StepType }) {
	const Icon = IconMap[type] ?? ArrowRight;

	return <Icon className="h-4 w-4" />;
}
