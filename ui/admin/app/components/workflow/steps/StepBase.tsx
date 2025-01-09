import { ChevronDown, ChevronRight, Trash } from "lucide-react";
import { useState } from "react";

import { Step, StepType, getDefaultStep } from "~/lib/model/workflows";
import { cn } from "~/lib/utils";

import {
	ConfirmationDialog,
	ConfirmationDialogProps,
} from "~/components/composed/ConfirmationDialog";
import { Button } from "~/components/ui/button";
import { useDndContext } from "~/components/ui/dnd";
import { SortableHandle } from "~/components/ui/dnd/sortable";
import { AutosizeTextarea } from "~/components/ui/textarea";
import { StepTypeSelect } from "~/components/workflow/steps/StepTypeSelect";
import { useConfirmationDialog } from "~/hooks/component-helpers/useConfirmationDialog";

const DeleteStepDialogProps: Partial<ConfirmationDialogProps> = {
	title: "Delete Step?",
	description:
		"Are you sure you want to delete this step? You will lose all data associated with it.",
	confirmProps: {
		children: "Delete",
		variant: "destructive",
	},
};

const UpdateStepTypeDialogProps: Partial<ConfirmationDialogProps> = {
	title: "Update Step Type?",
	description:
		"Are you sure you want to update the step type? This will reset all step data.",
	confirmProps: {
		children: "Update",
	},
};

export function StepBase({
	className,
	children,
	step,
	type,
	onUpdate,
	onDelete,
}: {
	className?: string;
	children: React.ReactNode;
	step: Step;
	type: StepType;
	onUpdate: (step: Step) => void;
	onDelete: () => void;
}) {
	const [isExpanded, setIsExpanded] = useState(false);

	const dnd = useDndContext();

	const showExpanded = isExpanded && !dnd.active;

	const fieldConfig = getTextFieldConfig();

	const { intercept, dialogProps } = useConfirmationDialog();

	const handleDelete = () => {
		if (hasContent()) {
			intercept(() => onDelete(), DeleteStepDialogProps);
		} else {
			onDelete();
		}
	};

	const handleUpdateType = (newType: StepType) => {
		if (newType === type) {
			return;
		}

		if (hasContent()) {
			intercept(
				() => onUpdate(getDefaultStep(newType)),
				UpdateStepTypeDialogProps
			);
		} else {
			onUpdate(getDefaultStep(newType));
		}
	};
	return (
		<div className={cn("rounded-md border bg-background", className)}>
			<div
				className={cn(
					"flex items-start gap-2 bg-background-secondary p-3",
					showExpanded ? "rounded-t-md" : "rounded-md"
				)}
			>
				<div className="flex items-center gap-2">
					<SortableHandle id={step.id} />

					<Button
						variant="ghost"
						size="icon"
						className="h-6 w-6 self-center p-0"
						onClick={(e) => {
							e.stopPropagation();
							setIsExpanded(!showExpanded);
						}}
					>
						{showExpanded ? (
							<ChevronDown className="h-4 w-4" />
						) : (
							<ChevronRight className="h-4 w-4" />
						)}
					</Button>

					<StepTypeSelect value={type} onChange={handleUpdateType} />
				</div>

				<AutosizeTextarea
					value={fieldConfig.value}
					onChange={(e) => fieldConfig.onChange(e.target.value)}
					placeholder={fieldConfig.placeholder}
					maxHeight={100}
					minHeight={0}
					className="flex-grow bg-background"
					onClick={(e) => e.stopPropagation()}
				/>

				<Button
					variant="ghost"
					size="icon"
					onClick={(e) => {
						e.stopPropagation();
						handleDelete();
					}}
				>
					<Trash className="h-4 w-4" />
				</Button>

				<ConfirmationDialog {...dialogProps} />
			</div>

			{showExpanded && children}
		</div>
	);

	function getTextFieldConfig() {
		if (type === "if" && step.if) {
			const copy = step.if;

			return {
				placeholder: "Condition",
				value: step.if.condition,
				onChange: (value: string) =>
					onUpdate({ ...step, if: { ...copy, condition: value } }),
			};
		} else if (type === "while" && step.while) {
			const copy = step.while;

			return {
				placeholder: "Condition",
				value: step.while.condition,
				onChange: (value: string) =>
					onUpdate({ ...step, while: { ...copy, condition: value } }),
			};
		}

		return {
			placeholder: "Step",
			value: step.step,
			onChange: (value: string) => onUpdate({ ...step, step: value }),
		};
	}

	function hasContent() {
		const { workflows, tools } = step;
		if (workflows.length || tools.length) {
			return true;
		}
		if (type === "if" && step.if) {
			return (
				step.if.condition.length || step.if.else.length || step.if.steps.length
			);
		} else if (type === "while" && step.while) {
			return step.while.condition.length || step.while.steps.length;
		}

		return step.step?.length;
	}
}
