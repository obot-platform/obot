import { ChevronRight, Trash } from "lucide-react";
import { memo, useState } from "react";

import { Step } from "~/lib/model/tasks";
import { cn } from "~/lib/utils";

import {
	ConfirmationDialog,
	ConfirmationDialogProps,
} from "~/components/composed/ConfirmationDialog";
import { Animate } from "~/components/ui/animate";
import { ExpandAndCollapse } from "~/components/ui/animate/expand";
import { Rotate } from "~/components/ui/animate/rotate";
import { SlideInOut } from "~/components/ui/animate/slide-in-out";
import { Button } from "~/components/ui/button";
import { useDndContext } from "~/components/ui/dnd";
import { SortableHandle } from "~/components/ui/dnd/sortable";
import { AutosizeTextarea } from "~/components/ui/textarea";
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

export const StepBase = memo(function StepBase({
	className,
	children,
	step,
	onUpdate,
	onDelete,
}: {
	className?: string;
	children?: React.ReactNode;
	step: Step;
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

	return (
		<SlideInOut direction={{ in: "up", out: "right" }}>
			<Animate.div
				className={cn("rounded-md border bg-background", className)}
				layout
				transition={dnd.active ? { duration: 0 } : undefined}
			>
				<div
					className={cn(
						"flex items-start gap-2 bg-background-secondary p-3",
						showExpanded ? "rounded-t-md" : "rounded-md"
					)}
				>
					<div className="flex items-center gap-2 self-center">
						<SortableHandle id={step.id} />

						{children && (
							<Button
								variant="ghost"
								size="icon"
								className="h-6 w-6 self-center p-0"
								onClick={(e) => {
									e.stopPropagation();
									setIsExpanded(!showExpanded);
								}}
							>
								<Rotate active={showExpanded}>
									<ChevronRight className="h-4 w-4" />
								</Rotate>
							</Button>
						)}
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

				<ExpandAndCollapse active={showExpanded}>{children}</ExpandAndCollapse>
			</Animate.div>
		</SlideInOut>
	);

	function getTextFieldConfig() {
		return {
			placeholder: "Step",
			value: step.step,
			onChange: (value: string) => onUpdate({ ...step, step: value }),
		};
	}

	function hasContent() {
		return step.step?.length;
	}
});
