import { zodResolver } from "@hookform/resolvers/zod";
import { ReactNode, useEffect, useMemo } from "react";
import { useFieldArray, useForm } from "react-hook-form";
import useSWR from "swr";
import { z } from "zod";

import { Agent } from "~/lib/model/agents";
import { ToolReferenceService } from "~/lib/service/api/toolreferenceService";
import { noop } from "~/lib/utils";

import { ToolEntry } from "~/components/agent/ToolEntry";
import { ToolCatalogDialog } from "~/components/tools/ToolCatalog";
import { Animate, AnimatePresence } from "~/components/ui/animate";
import { SlideInOut } from "~/components/ui/animate/slide-in-out";
import { Form } from "~/components/ui/form";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "~/components/ui/select";
import { useCapabilityTools } from "~/hooks/tools/useCapabilityTools";

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
	oauthApps: z.array(z.string()),
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
			oauthApps: agent.oauthApps ?? [],
		};
	}, [agent]);

	const form = useForm<ToolFormValues>({
		resolver: zodResolver(formSchema),
		defaultValues,
	});
	const { control, handleSubmit, getValues, reset } = form;

	const { data: toolList } = useSWR(
		ToolReferenceService.getToolReferences.key("tool"),
		() => ToolReferenceService.getToolReferences("tool"),
		{ fallbackData: [] }
	);

	const oauthToolMap = useMemo(
		() => new Map(toolList.map((tool) => [tool.id, tool.metadata?.oauth])),
		[toolList]
	);

	useEffect(() => {
		const unchangedTools = compareArrays(
			defaultValues.tools.map((x) => x.tool),
			getValues("tools").map((x) => x.tool)
		);
		const unchangedOauths = compareArrays(
			defaultValues.oauthApps,
			getValues("oauthApps")
		);

		if (unchangedTools && unchangedOauths) return;

		reset(defaultValues);
	}, [defaultValues, reset, getValues]);

	const toolFields = useFieldArray<ToolFormValues>({
		control,
		name: "tools",
	});

	const removeTool = (toolId: string, oauthToRemove?: string) => {
		const updatedTools = toolFields.fields.filter(
			(tool) => tool.tool !== toolId
		);
		const index = toolFields.fields.findIndex((tool) => tool.tool === toolId);
		toolFields.remove(index);

		const stillHasOauth = updatedTools.some(
			(tool) => oauthToolMap.get(tool.tool) === oauthToRemove
		);

		if (!stillHasOauth) {
			const updatedOauths = form
				.getValues("oauthApps")
				?.filter((oauth) => oauth !== oauthToRemove);
			form.setValue("oauthApps", updatedOauths);
		}
		onChange?.(form.getValues());
	};

	const updateVariant = (tool: string, variant: ToolVariant) => {
		toolFields.update(
			toolFields.fields.findIndex((t) => t.tool === tool),
			{ tool, variant }
		);
		onChange?.(form.getValues());
	};

	const updateTools = (
		tools: string[],
		variant: ToolVariant,
		oauths: string[]
	) => {
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

		form.setValue("oauthApps", oauths);
		onChange?.(form.getValues());
	};

	const getCapabilities = useCapabilityTools();
	const capabilities = new Set(getCapabilities.data?.map((x) => x.id));

	const sortedFields = toolFields.fields
		.filter((field) => !capabilities.has(field.tool))
		.toSorted((a, b) => a.tool.localeCompare(b.tool));

	return (
		<Form {...form}>
			<form
				onSubmit={handleSubmit(onSubmit || noop)}
				className="flex flex-col gap-4"
			>
				<div className="mt-2 w-full">
					<AnimatePresence mode="popLayout">
						{sortedFields.map((field) => (
							<SlideInOut
								key={field.tool}
								direction={{ in: "left", out: "right" }}
								layout
							>
								<ToolEntry
									tool={field.tool}
									onDelete={removeTool}
									actions={
										<>
											<Select
												value={field.variant}
												onValueChange={(value: ToolVariant) =>
													updateVariant(field.tool, value)
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
							</SlideInOut>
						))}
					</AnimatePresence>
				</div>

				<Animate.div layout className="flex justify-end">
					<ToolCatalogDialog
						tools={toolFields.fields.map((field) => field.tool)}
						onUpdateTools={(tools, oauths) => {
							updateTools(tools, ToolVariant.FIXED, oauths);
						}}
						oauths={form.watch("oauthApps")}
					/>
				</Animate.div>
			</form>
		</Form>
	);
}

function compareArrays(a: string[], b: string[]) {
	const aSet = new Set(a);

	if (aSet.size !== b.length) return false;

	return b.every((tool) => aSet.has(tool));
}
