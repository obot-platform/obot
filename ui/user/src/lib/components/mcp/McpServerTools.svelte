<script lang="ts">
	import { ChatService, type MCPServerTool, type Project, type ProjectMCP } from '$lib/services';
	import { ChevronDown, ChevronsRight, ChevronUp, LoaderCircle, Server } from 'lucide-svelte';
	import Toggle from '../Toggle.svelte';
	import { onMount, type Snippet } from 'svelte';
	import { fade, slide } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

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
	let allDescriptionsEnabled = $state(false);
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
	<div class="relative flex flex-col gap-4 p-4 pb-0">
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
						{mcpServer.name}
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
				<div class="flex justify-end gap-4 border-r border-transparent pr-3">
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

					<div class="bg-surface3 h-5 w-0.5"></div>

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
										onclick={() => {
											expandedDescriptions[tool.id] = !expandedDescriptions[tool.id];
										}}
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
			'dark:bg-surface2 sticky bottom-0 left-0 flex w-full justify-end bg-white p-4',
			classes?.actions
		)}
	>
		<button class="button-primary flex items-center gap-1" onclick={handleSubmit}>
			{submitText}
			<ChevronsRight class="size-4" />
		</button>
	</div>
</div>
