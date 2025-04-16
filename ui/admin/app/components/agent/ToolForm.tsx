import { zodResolver } from "@hookform/resolvers/zod";
import { ReactNode, useEffect, useMemo } from "react";
import { useFieldArray, useForm } from "react-hook-form";
import useSWR from "swr";
import { z } from "zod";

import { Agent } from "~/lib/model/agents";
import { ToolReference } from "~/lib/model/toolReferences";
import { ToolReferenceService } from "~/lib/service/api/toolreferenceService";
import { noop } from "~/lib/utils";

import { ToolEntry } from "~/components/agent/ToolEntry";
import { ToolCatalogDialog } from "~/components/tools/ToolCatalog";
import {
	Accordion,
	AccordionContent,
	AccordionItem,
	AccordionTrigger,
} from "~/components/ui/accordion";
import { Animate, AnimatePresence } from "~/components/ui/animate";
import { Form } from "~/components/ui/form";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "~/components/ui/select";
import { useSlideInOut } from "~/hooks/animate/useSlideInOut";
import { useCapabilityTools } from "~/hooks/tools/useCapabilityTools";

export const ToolVariant = {
	FIXED: "fixed",
	DEFAULT: "default",
	AVAILABLE: "available",
	OFF: "off",
} as const;
export type ToolVariant = (typeof ToolVariant)[keyof typeof ToolVariant];
const formSchema = z.object({
	tools: z.array(
		z.object({
			tool: z.string(),
			variant: z.enum([
				ToolVariant.FIXED,
				ToolVariant.DEFAULT,
				ToolVariant.AVAILABLE,
				ToolVariant.OFF,
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

	const toolMap = useMemo(() => {
		return new Map(toolList.map((tool) => [tool.id, tool]));
	}, [toolList]);

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

	const getField = (tool: string) => {
		const field = toolFields.fields.find((field) => field.tool === tool);
		if (!field) throw new Error(`Field not found: ${tool}`);
		return field;
	};

	const toolGroups = useMemo(() => {
		const groups = new Map<string, { name: string; tools: string[] }>();

		for (const field of sortedFields) {
			const tool = toolMap.get(field.tool);
			if (!tool) continue;

			const bundleToolName = tool.bundleToolName ?? tool.id;
			const group = groups.get(bundleToolName) ?? {
				name: bundleToolName,
				tools: [],
			};

			// make sure bundle tool is first
			if (tool.bundle) {
				group.tools.unshift(tool.id);
			} else {
				group.tools.push(tool.id);
			}
			groups.set(bundleToolName, group);
		}

		return groups;
	}, [sortedFields, toolMap]);

	const { exit: slideExit } = useSlideInOut({ direction: "right" });

	return (
		<Form {...form}>
			<form
				onSubmit={handleSubmit(onSubmit || noop)}
				className="flex flex-col gap-4"
			>
				<div className="mt-2 w-full">
					<Accordion type="multiple">
						<AnimatePresence mode="popLayout">
							{Array.from(toolGroups.values()).map((group) => {
								const bundleTool = toolMap.get(group.name);
								const bundleTools = group.tools
									.map((tool) => toolMap.get(tool))
									.filter((x) => !!x);

								if (!bundleTool || !bundleTools.length) return null;

								return (
									<Animate.div key={group.name} layout exit={slideExit}>
										<AccordionItem value={group.name}>
											<AccordionTrigger value={group.name}>
												<ToolEntry tool={group.name} />
											</AccordionTrigger>

											<AccordionContent className="flex flex-col gap-2 px-2">
												<AnimatePresence mode="popLayout">
													{bundleTools.map(renderToolEntry)}
												</AnimatePresence>
											</AccordionContent>
										</AccordionItem>
									</Animate.div>
								);
							})}
						</AnimatePresence>
					</Accordion>
				</div>

				<Animate.div layout className="flex justify-end">
					<ToolCatalogDialog
						tools={toolFields.fields.map((field) => field.tool)}
						onUpdateTools={(tools, oauths) => {
							updateTools(tools, ToolVariant.AVAILABLE, oauths);
						}}
						oauths={form.watch("oauthApps")}
					/>
				</Animate.div>
			</form>
		</Form>
	);

	function renderToolEntry(tool: ToolReference) {
		const field = getField(tool.id);
		return (
			<Animate.div key={field.tool} exit={slideExit} layout>
				<ToolEntry
					bundleBadge
					key={field.tool}
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
									<SelectItem value={ToolVariant.FIXED}>Always On</SelectItem>
									<SelectItem value={ToolVariant.DEFAULT}>
										<p>
											Optional
											<span className="text-muted-foreground">{" - On"}</span>
										</p>
									</SelectItem>
									<SelectItem value={ToolVariant.AVAILABLE}>
										<p>
											Optional
											<span className="text-muted-foreground">{" - Off"}</span>
										</p>
									</SelectItem>
								</SelectContent>
							</Select>

							{renderActions?.(field.tool)}
						</>
					}
				/>
			</Animate.div>
		);
	}
}

function compareArrays(a: string[], b: string[]) {
	const aSet = new Set(a);

	if (aSet.size !== b.length) return false;

	return b.every((tool) => aSet.has(tool));
}
