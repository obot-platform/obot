<script lang="ts">
	import { ChatService, type MCPServerTool, type Project, type ProjectMCP } from '$lib/services';
	import { ChevronDown, ChevronsRight, ChevronUp, LoaderCircle, Server } from 'lucide-svelte';
	import Toggle from '../Toggle.svelte';
	import { onMount, type Snippet } from 'svelte';
	import { fade, slide } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';
	import { DEFAULT_CUSTOM_SERVER_NAME } from '$lib/constants';
	import { responsive } from '$lib/stores';

	interface Props {
		mcpServer: ProjectMCP;
		project: Project;
		header?: Snippet;
		onSubmit?: (selected: string[]) => void;
		submitText?: string;
		currentThreadID?: string;
		tools?: MCPServerTool[];
		isNew?: boolean;
		classes?: {
			actions?: string;
		};
	}

	const {
		mcpServer,
		project,
		header,
		onSubmit,
		currentThreadID,
		tools: refTools,
		submitText = 'Continue',
		isNew,
		classes
	}: Props = $props();

	let tools = $state<MCPServerTool[]>(refTools ?? []);
	let selected = $state<string[]>(['*']);
	let allToolsEnabled = $derived(selected[0] === '*' || selected.length === tools.length);
	let loading = $state(false);
	let expandedDescriptions = $state<Record<string, boolean>>({});
	let expandedParams = $state<Record<string, boolean>>({});
	let allDescriptionsEnabled = $state(true);
	let allParamsEnabled = $state(false);
	$effect(() => {
		if (refTools) {
			tools = refTools;
			selected = isNew ? ['*'] : tools.filter((t) => t.enabled).map((t) => t.id);
		}
	});

	onMount(async () => {
		loading = true;
		if (refTools) {
			tools = refTools;
		} else if (!refTools && project && mcpServer) {
			tools = currentThreadID
				? await ChatService.listProjectThreadMcpServerTools(
						project.assistantID,
						project.id,
						mcpServer.id,
						currentThreadID
					)
				: await ChatService.listProjectMCPServerTools(
						project.assistantID,
						project.id,
						mcpServer.id
					);
		}
		selected = isNew ? ['*'] : tools.filter((t) => t.enabled).map((t) => t.id);
		loading = false;
	});

	function handleToggleDescription(toolId: string) {
		if (allDescriptionsEnabled) {
			allDescriptionsEnabled = false;
			for (const { id: refToolId } of tools) {
				if (toolId !== refToolId) {
					expandedDescriptions[refToolId] = true;
				}
			}
			expandedDescriptions[toolId] = false;
		} else {
			expandedDescriptions[toolId] = !expandedDescriptions[toolId];
		}

		const expandedDescriptionValues = Object.values(expandedDescriptions);
		if (
			expandedDescriptionValues.length === tools.length &&
			expandedDescriptionValues.every((v) => v)
		) {
			allDescriptionsEnabled = true;
		}
	}

	async function handleSubmit() {
		if (currentThreadID) {
			await ChatService.configureProjectThreadMcpServerTools(
				project.assistantID,
				project.id,
				mcpServer.id,
				currentThreadID,
				selected
			);
		} else {
			await ChatService.configureProjectMcpServerTools(
				project.assistantID,
				project.id,
				mcpServer.id,
				selected
			);
		}

		onSubmit?.(selected);
	}
</script>

<div class="flex h-full flex-col gap-4">
	<div class="relative flex flex-col gap-4 pb-0 md:p-4">
		{#if header}
			{@render header()}
		{:else}
			<h2 class="flex text-xl font-semibold">
				{currentThreadID ? 'Modify Thread Tools' : 'Modify Server Tools'}
			</h2>
		{/if}
		<div class="mb-4 flex flex-col gap-1">
			<div class="flex items-center gap-2">
				<div class="h-fit flex-shrink-0 self-start rounded-md bg-gray-50 p-1 dark:bg-gray-600">
					{#if mcpServer.icon}
						<img src={mcpServer.icon} alt={mcpServer.name} class="size-6" />
					{:else}
						<Server class="size-6" />
					{/if}
				</div>
				<div class="flex flex-col gap-1">
					<h3 class="text-lg leading-5.5 font-semibold">
						{mcpServer.name || DEFAULT_CUSTOM_SERVER_NAME}
					</h3>
				</div>
			</div>
			<p class="text-sm font-light text-gray-500">
				{mcpServer.description}
			</p>
		</div>

		{#if loading}
			<div class="flex items-center justify-center">
				<LoaderCircle class="size-6 animate-spin" />
			</div>
		{:else}
			<div in:fade class="flex flex-col gap-2">
				<div class="flex flex-wrap justify-end gap-4 border-r border-transparent pr-3">
					<Toggle
						checked={allDescriptionsEnabled}
						onChange={(checked) => {
							allDescriptionsEnabled = checked;
							expandedDescriptions = {};
						}}
						label="Show All Descriptions"
						labelInline
						classes={{
							label: 'text-sm gap-2'
						}}
					/>

					{#if !responsive.isMobile}
						<div class="bg-surface3 h-5 w-0.5"></div>
					{/if}

					<Toggle
						checked={allParamsEnabled}
						onChange={(checked) => {
							allParamsEnabled = checked;
							expandedParams = {};
						}}
						label="Show All Parameters"
						labelInline
						classes={{
							label: 'text-sm gap-2'
						}}
					/>

					{#if !responsive.isMobile}
						<div class="bg-surface3 h-5 w-0.5"></div>
					{/if}

					<Toggle
						checked={allToolsEnabled}
						onChange={(checked) => {
							selected = checked ? ['*'] : [];
						}}
						label="Enable All Tools"
						labelInline
						classes={{
							label: 'text-sm gap-2'
						}}
					/>
				</div>
				<div class="flex flex-col gap-2 overflow-hidden">
					{#each tools as tool}
						<div
							class="border-surface2 dark:border-surface3 flex flex-col gap-2 rounded-md border p-3"
						>
							<div class="flex items-center justify-between gap-2">
								<p class="text-md font-semibold">{tool.name}</p>
								<div class="flex flex-shrink-0 items-center gap-2">
									<button
										class="icon-button h-fit min-h-auto w-fit min-w-auto flex-shrink-0 p-1"
										onclick={() => handleToggleDescription(tool.id)}
									>
										{#if expandedDescriptions[tool.id]}
											<ChevronUp class="size-4" />
										{:else}
											<ChevronDown class="size-4" />
										{/if}
									</button>
									<Toggle
										checked={selected.includes(tool.id) || allToolsEnabled}
										onChange={(checked) => {
											if (allToolsEnabled) {
												selected = tools.map((t) => t.id).filter((id) => id !== tool.id);
											} else {
												selected = checked
													? [...selected, tool.id]
													: selected.filter((id) => id !== tool.id);
											}
										}}
										label="On/Off"
										disablePortal
									/>
								</div>
							</div>
							{#if expandedDescriptions[tool.id] || allDescriptionsEnabled}
								<p in:slide={{ axis: 'y' }} class="text-sm font-light text-gray-500">
									{tool.description}
								</p>
								{#if Object.keys(tool.params ?? {}).length > 0}
									{#if expandedParams[tool.id] || allParamsEnabled}
										<div
											class={'from-surface2 dark:from-surface3 flex w-full flex-shrink-0 bg-linear-to-r to-transparent px-4 py-2 text-xs font-semibold text-gray-500 md:w-sm'}
										>
											Parameters
										</div>
										<div class="flex flex-col px-4 text-xs" in:slide={{ axis: 'y' }}>
											<div class="flex flex-col gap-2">
												{#each Object.keys(tool.params ?? {}) as paramKey}
													<div class="flex flex-col items-center gap-2 md:flex-row">
														<p class="self-start font-semibold text-gray-500 md:min-w-xs">
															{paramKey}
														</p>
														<p class="self-start font-light text-gray-500">
															{tool.params?.[paramKey]}
														</p>
													</div>
												{/each}
											</div>
										</div>
									{/if}
								{/if}
							{/if}
						</div>
					{/each}
				</div>
			</div>
		{/if}
	</div>
	<div class="flex grow"></div>
	<div
		class={twMerge(
			'dark:bg-surface2 sticky bottom-0 left-0 flex w-full justify-end bg-white py-4 md:px-4',
			classes?.actions
		)}
	>
		<button class="button-primary flex items-center gap-1" onclick={handleSubmit}>
			{submitText}
			<ChevronsRight class="size-4" />
		</button>
	</div>
</div>
