import { ChevronDown, ChevronRight, Trash } from "lucide-react";
import { useState } from "react";

import { Step, Template } from "~/lib/model/workflows";
import { cn } from "~/lib/utils";

import { Button } from "~/components/ui/button";
import { Textarea } from "~/components/ui/textarea";

export function TemplateComponent({
	step,
	onUpdate,
	onDelete,
	className,
}: {
	step: Step;
	onUpdate: (updatedStep: Step) => void;
	onDelete: () => void;
	className?: string;
}) {
	const [isExpanded, setIsExpanded] = useState(false);

	if (!step.template) {
		console.error("TemplateComponent received a step without a template");
		return null;
	}

	const renderTemplateArgs = (template: Template) => {
		return Object.entries(template.args).map(([key, value]) => {
			return (
				<div key={key} className="mb-4">
					<label htmlFor={key} className="mb-1 block text-sm font-medium">
						{key}
					</label>
					<Textarea
						id={key}
						placeholder={value}
						value={value}
						onChange={(e) => {
							const updatedTemplate = {
								...template,
								args: {
									...template.args,
									[key]: e.target.value,
								},
							};
							onUpdate({ ...step, template: updatedTemplate });
						}}
						className="bg-background"
					/>
				</div>
			);
		});
	};

	return (
		<div className={cn("rounded-md border", className)}>
			<div
				className={cn(
					"flex items-center bg-secondary p-3",
					isExpanded ? "rounded-t-md" : "rounded-md"
				)}
			>
				<Button
					variant="ghost"
					size="sm"
					className="mr-2 h-6 w-6 p-0"
					onClick={() => setIsExpanded(!isExpanded)}
				>
					{isExpanded ? (
						<ChevronDown className="h-4 w-4" />
					) : (
						<ChevronRight className="h-4 w-4" />
					)}
				</Button>
				<div className="flex-grow rounded-md border bg-background p-2">
					{step.name}
				</div>
				<Button
					variant="destructive"
					size="icon"
					onClick={onDelete}
					className="ml-2"
				>
					<Trash className="h-4 w-4" />
				</Button>
			</div>
			{isExpanded && (
				<div className="space-y-4 p-3 px-8">
					<div className="mb-4">{renderTemplateArgs(step.template)}</div>
				</div>
			)}
		</div>
	);
}
