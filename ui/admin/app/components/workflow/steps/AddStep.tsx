import { PlusIcon } from "lucide-react";

import { Step, getDefaultStep } from "~/lib/model/workflows";

import { Button } from "~/components/ui/button";

interface AddStepButtonProps {
	onAddStep: (newStep: Step) => void;
	className?: string;
}

export function AddStepButton({ onAddStep, className }: AddStepButtonProps) {
	return (
		<Button
			variant="ghost"
			className={className}
			startContent={<PlusIcon />}
			onClick={() => onAddStep(getDefaultStep())}
		>
			Add Step
		</Button>
	);
}
