import { zodResolver } from "@hookform/resolvers/zod";
import { ReactNode, useEffect, useMemo } from "react";
import { useFieldArray, useForm } from "react-hook-form";
import { z } from "zod";

import { Agent } from "~/lib/model/agents";
import { noop } from "~/lib/utils";

import { ToolEntry } from "~/components/agent/ToolEntry";
import { ToolCatalogDialog } from "~/components/tools/ToolCatalog";
import { Form } from "~/components/ui/form";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "~/components/ui/select";

const ToolVariant = {
	FIXED: "fixed",
	DEFAULT: "default",
	AVAILABLE: "available",
} as const;
type ToolVariant = (typeof ToolVariant)[keyof typeof ToolVariant];
const formSchema = z.object({
	tools: z.array(
		z.object({
			tool: z.string(),
			variant: z.enum([
				ToolVariant.FIXED,
				ToolVariant.DEFAULT,
				ToolVariant.AVAILABLE,
			] as const),
		})
	),
});

export type ToolFormValues = z.infer<typeof formSchema>;

export function ToolForm({
	agent,
	onSubmit,
	onChange,
	renderActions,
}: {
	agent: Agent;
	onSubmit?: (values: ToolFormValues) => void;
	onChange?: (values: ToolFormValues) => void;
	renderActions?: (tool: string) => ReactNode;
}) {
	const defaultValues = useMemo(() => {
		return {
			tools: [
				...(agent.tools ?? []).map((tool) => ({
					tool,
					variant: ToolVariant.FIXED,
				})),
				...(agent.defaultThreadTools ?? []).map((tool) => ({
					tool,
					variant: ToolVariant.DEFAULT,
				})),
				...(agent.availableThreadTools ?? []).map((tool) => ({
					tool,
					variant: ToolVariant.AVAILABLE,
				})),
			],
		};
	}, [agent]);

	const form = useForm<ToolFormValues>({
		resolver: zodResolver(formSchema),
		defaultValues,
	});
	const { control, handleSubmit, getValues, reset, watch } = form;

	useEffect(() => {
		const unchanged = compareArrays(
			defaultValues.tools.map((x) => x.tool),
			getValues("tools").map((x) => x.tool)
		);

		if (unchanged) return;

		reset(defaultValues);
	}, [defaultValues, reset, getValues]);

	const toolFields = useFieldArray({
		control,
		name: "tools",
	});

	useEffect(() => {
		return watch((values) => {
			const { data, success } = formSchema.safeParse(values);

			if (!success) return;

			onChange?.(data);
		}).unsubscribe;
	}, [watch, onChange]);

	const removeTools = (tools: string[]) => {
		const indexes = tools
			.map((tool) => toolFields.fields.findIndex((t) => t.tool === tool))
			.filter((index) => index !== -1);

		toolFields.remove(indexes);
	};

	const updateVariant = (tool: string, variant: ToolVariant) =>
		toolFields.update(
			toolFields.fields.findIndex((t) => t.tool === tool),
			{ tool, variant }
		);

	const updateTools = (tools: string[], variant: ToolVariant) => {
		const removedToolIndexes = toolFields.fields
			.filter((field) => !tools.includes(field.tool))
			.map((item) => toolFields.fields.indexOf(item));

		const addedTools = tools.filter(
			(tool) => !toolFields.fields.some((field) => field.tool === tool)
		);

		toolFields.remove(removedToolIndexes);

		for (const tool of addedTools) {
			toolFields.append({ tool, variant });
		}
	};

	const sortedFields = toolFields.fields.toSorted((a, b) =>
		a.tool.localeCompare(b.tool)
	);

	return (
		<Form {...form}>
			<form
				onSubmit={handleSubmit(onSubmit || noop)}
				className="flex flex-col gap-4"
			>
				<div className="mt-2 w-full overflow-y-auto">
					{sortedFields.map((field) => (
						<ToolEntry
							key={field.id}
							tool={field.tool}
							onDelete={() => removeTools([field.tool])}
							actions={
								<>
									<Select
										value={
											field.variant === ToolVariant.FIXED
												? field.variant
												: ToolVariant.DEFAULT
										}
										onValueChange={(value) =>
											updateVariant(field.tool, value as ToolVariant)
										}
									>
										<SelectTrigger className="w-36">
											<SelectValue />
										</SelectTrigger>

										<SelectContent>
											<SelectItem value={ToolVariant.FIXED}>
												Always On
											</SelectItem>
											<SelectItem value={ToolVariant.DEFAULT}>
												<p>
													Optional
													<span className="text-muted-foreground">
														{" - On"}
													</span>
												</p>
											</SelectItem>
											<SelectItem value={ToolVariant.AVAILABLE}>
												<p>
													Optional
													<span className="text-muted-foreground">
														{" - Off"}
													</span>
												</p>
											</SelectItem>
										</SelectContent>
									</Select>

									{renderActions?.(field.tool)}
								</>
							}
						/>
					))}
				</div>

				<div className="flex justify-end">
					<ToolCatalogDialog
						tools={toolFields.fields.map((field) => field.tool)}
						onUpdateTools={(tools) => updateTools(tools, ToolVariant.FIXED)}
					/>
				</div>
			</form>
		</Form>
	);
}

function compareArrays(a: string[], b: string[]) {
	const aSet = new Set(a);

	if (aSet.size !== b.length) return false;

	return b.every((tool) => aSet.has(tool));
}
