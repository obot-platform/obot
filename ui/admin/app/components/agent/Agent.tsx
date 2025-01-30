import { GearIcon } from "@radix-ui/react-icons";
import { BlocksIcon, LibraryIcon, PlusIcon, WrenchIcon } from "lucide-react";
import { useCallback, useEffect, useState } from "react";

import { Agent as AgentType } from "~/lib/model/agents";
import { AssistantNamespace } from "~/lib/model/assistants";
import { cn } from "~/lib/utils";

import { AgentAlias } from "~/components/agent/AgentAlias";
import { useAgent } from "~/components/agent/AgentContext";
import { AgentForm } from "~/components/agent/AgentForm";
import { AgentIntroForm } from "~/components/agent/AgentIntroForm";
import { PastThreads } from "~/components/agent/PastThreads";
import { ToolForm } from "~/components/agent/ToolForm";
import { AgentCapabilityForm } from "~/components/agent/shared/AgentCapabilityForm";
import { AgentModelSelect } from "~/components/agent/shared/AgentModelSelect";
import { EnvironmentVariableSection } from "~/components/agent/shared/EnvironmentVariableSection";
import { ToolAuthenticationStatus } from "~/components/agent/shared/ToolAuthenticationStatus";
import { WorkspaceFilesSection } from "~/components/agent/shared/WorkspaceFilesSection";
import { AgentKnowledgePanel } from "~/components/knowledge";
import {
	Accordion,
	AccordionContent,
	AccordionItem,
	AccordionTrigger,
} from "~/components/ui/accordion";
import { Button } from "~/components/ui/button";
import { CardDescription } from "~/components/ui/card";
import { ScrollArea } from "~/components/ui/scroll-area";
import { useDebounce } from "~/hooks/useDebounce";

type AgentProps = {
	className?: string;
	currentThreadId?: string | null;
	onRefresh?: (threadId: string | null) => void;
};

export function Agent({ className, currentThreadId, onRefresh }: AgentProps) {
	const { agent, updateAgent, refreshAgent, isUpdating, lastUpdated, error } =
		useAgent();

	const [agentUpdates, setAgentUpdates] = useState(agent);
	const [enableScrollStick, setEnableScrollStick] = useState(false);

	useEffect(() => {
		setAgentUpdates((prev) => {
			return {
				...agent,
				aliasAssigned:
					agent.aliasAssigned !== undefined
						? agent.aliasAssigned
						: prev.aliasAssigned,
			};
		});
	}, [agent]);

	const debouncedUpdateAgent = useDebounce(updateAgent, 1000);

	const partialSetAgent = useCallback(
		(changes: Partial<typeof agent>) => {
			const updatedAgent = { ...agent, ...agentUpdates, ...changes };

			debouncedUpdateAgent(updatedAgent);

			setAgentUpdates(updatedAgent);

			if (changes.alias) {
				const updatedAgentWithAliasUndefined = {
					...updatedAgent,
					aliasAssigned: undefined,
				};
				setAgentUpdates(updatedAgentWithAliasUndefined);
			} else {
				setAgentUpdates(updatedAgent);
			}
		},
		[agent, agentUpdates, debouncedUpdateAgent]
	);

	const handleThreadSelect = useCallback(
		(threadId: string) => {
			onRefresh?.(threadId);
		},
		[onRefresh]
	);

	const handleAccordionValueChange = useCallback((value: string[]) => {
		setEnableScrollStick(value.includes("model"));
	}, []);

	return (
		<div className="flex h-full flex-col">
			<ScrollArea
				className={cn("h-full", className)}
				enableScrollStick={enableScrollStick ? "bottom" : undefined}
			>
				<AgentAlias agent={agentUpdates} onChange={partialSetAgent} />

				<div className="m-4 p-4">
					<AgentForm agent={agentUpdates} onChange={partialSetAgent} />
				</div>

				<div className="m-4 p-4">
					<AgentIntroForm agent={agentUpdates} onChange={partialSetAgent} />
				</div>

				<div className="m-4 space-y-4 p-4">
					<h4 className="flex items-center gap-2 border-b pb-2">
						<BlocksIcon />
						Capabilities
					</h4>

					<CardDescription>
						Capabilities define how users can interact with this agent in the
						chat interface. Each capability enables specific features that users
						can access when using the agent.
					</CardDescription>

					<AgentCapabilityForm
						entity={agentUpdates}
						onChange={partialSetAgent}
					/>
				</div>

				<div className="m-4 space-y-4 p-4">
					<h4 className="flex items-center gap-2 border-b pb-2">
						<WrenchIcon />
						Tools
					</h4>

					<CardDescription>
						Add tools that allow the agent to perform useful actions such as
						searching the web, reading files, or interacting with other systems.
					</CardDescription>

					<ToolForm
						agent={agentUpdates}
						onChange={({ tools, oauthApps }) =>
							partialSetAgent(convertTools(tools, oauthApps))
						}
						renderActions={renderActions}
					/>
				</div>

				<div className="m-4 space-y-4 p-4">
					<h4 className="flex items-center gap-2 border-b pb-2">
						<LibraryIcon />
						Knowledge
					</h4>

					<CardDescription>
						Provide knowledge to the agent in the form of files, website, or
						external links in order to give it context about various topics.
					</CardDescription>

					<AgentKnowledgePanel
						agentId={agent.id}
						agent={agent}
						updateAgent={partialSetAgent}
						addTool={(tool) => {
							if (agent?.tools?.includes(tool)) return;

							partialSetAgent({
								tools: [...(agent.tools || []), tool],
							});
						}}
					/>
				</div>

				<WorkspaceFilesSection entityId={agent.id} />

				<Accordion
					type="multiple"
					className="m-4 p-4"
					onValueChange={handleAccordionValueChange}
				>
					<AccordionItem value="model">
						<AccordionTrigger className="border-b">
							<h4 className="flex items-center gap-2">
								<GearIcon className="size-5" />
								Advanced
							</h4>
						</AccordionTrigger>

						<AccordionContent className="space-y-8 py-4">
							<div className="flex flex-col gap-4">
								<h4>Model</h4>

								<CardDescription>
									The model to use for the agent.
								</CardDescription>

								<AgentModelSelect
									entity={agentUpdates}
									onChange={(updates) => partialSetAgent(updates)}
								/>
							</div>

							<EnvironmentVariableSection
								entity={agent}
								onUpdate={partialSetAgent}
								entityType="agent"
							/>
						</AccordionContent>
					</AccordionItem>
				</Accordion>
			</ScrollArea>

			<footer className="flex items-center justify-between gap-4 px-8 py-4 shadow-inner">
				<div className="text-muted-foreground">
					{error ? (
						<p>Error saving agent</p>
					) : isUpdating ? (
						<p>Saving...</p>
					) : lastUpdated ? (
						<p>Saved</p>
					) : (
						<div />
					)}
				</div>

				<div className="flex gap-2">
					<PastThreads
						currentThreadId={currentThreadId}
						agentId={agent.id}
						onThreadSelect={handleThreadSelect}
					/>

					<Button
						variant="outline"
						className="flex gap-2"
						onClick={() => {
							onRefresh?.(null);
						}}
					>
						<PlusIcon />
						New Thread
					</Button>
				</div>
			</footer>
		</div>
	);

	function renderActions(tool: string) {
		return (
			<ToolAuthenticationStatus
				namespace={AssistantNamespace.Agents}
				entityId={agent.id}
				tool={tool}
				toolInfo={agent.toolInfo?.[tool]}
				onUpdate={() => refreshAgent()}
			/>
		);
	}
}

function convertTools(
	tools: { tool: string; variant: "fixed" | "default" | "available" }[],
	oauthApps: string[]
) {
	type ToolObj = Pick<
		AgentType,
		"tools" | "defaultThreadTools" | "availableThreadTools" | "oauthApps"
	>;

	const toolsUpdate = tools.reduce(
		(acc, { tool, variant }) => {
			if (variant === "fixed") acc.tools?.push(tool);
			else if (variant === "default") acc.defaultThreadTools?.push(tool);
			else if (variant === "available") acc.availableThreadTools?.push(tool);

			return acc;
		},
		{
			tools: [],
			defaultThreadTools: [],
			availableThreadTools: [],
		} as ToolObj
	);

	return { ...toolsUpdate, oauthApps };
}
