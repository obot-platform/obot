<script lang="ts">
	import { ChatService, type MCPServerTool, type Project, type ProjectMCP } from '$lib/services';
	import { ChevronsRight, LoaderCircle, Server } from 'lucide-svelte';
	import Toggle from '../Toggle.svelte';
	import { onMount, type Snippet } from 'svelte';
	import { fade } from 'svelte/transition';

	interface Props {
		mcpServer: ProjectMCP;
		project: Project;
		header?: Snippet;
		onSubmit?: (selected: string[]) => void;
		submitText?: string;
		currentThreadID?: string;
		tools?: MCPServerTool[];
		isNew?: boolean;
	}

	const {
		mcpServer,
		project,
		header,
		onSubmit,
		currentThreadID,
		tools: refTools,
		submitText = 'Continue',
		isNew
	}: Props = $props();

	let tools = $state<MCPServerTool[]>(refTools ?? []);
	let selected = $state<string[]>(['*']);
	let allToolsEnabled = $derived(selected[0] === '*' || selected.length === tools.length);
	let loading = $state(false);

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
		<div class="flex flex-col gap-1">
			<div class="flex max-w-sm items-center gap-2">
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
				<div class="flex justify-end border-r border-transparent pr-3">
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
						<div class="border-surface2 flex flex-col gap-2 rounded-md border p-3">
							<div class="flex items-center justify-between gap-2">
								<p class="text-md font-semibold">{tool.name}</p>
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
									label={`Turn on/off ${tool.name}`}
								/>
							</div>
							<p class="text-sm font-light text-gray-500">
								{tool.description}
							</p>
						</div>
					{/each}
				</div>
			</div>
		{/if}
	</div>
	<div class="flex grow"></div>
	<div class="dark:bg-surface2 sticky bottom-0 left-0 flex w-full justify-end bg-white p-4">
		<button class="button-primary flex items-center gap-1" onclick={handleSubmit}>
			{submitText}
			<ChevronsRight class="size-4" />
		</button>
	</div>
</div>
