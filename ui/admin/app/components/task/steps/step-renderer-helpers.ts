import type { Step } from "~/lib/model/tasks";

export type StepRendererProps = {
	step: Step;
	onUpdate: (updatedStep: Step) => void;
	onDelete: () => void;
};

export type StepRenderer = (props: StepRendererProps) => React.ReactNode;
