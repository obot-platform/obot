<script lang="ts">
	import { LoaderCircle, ChevronDown, ChevronUp, Wrench } from 'lucide-svelte';
	import Toggle from '../Toggle.svelte';
	import { slide } from 'svelte/transition';
	import Search from '../Search.svelte';

	interface ToolMapping {
		id: string;
		serverName: string;
		originalName: string;
		displayName: string;
		originalDescription: string;
		displayDescription: string;
		enabled: boolean;
		parameters?: ToolParameter[];
	}

	interface ToolParameter {
		name: string;
		displayName: string;
		description: string;
		displayDescription: string;
		type: string;
		required: boolean;
	}

	interface Props {
		compositeConfig: { componentCatalogEntries: string[] };
		catalogId?: string;
		readonly?: boolean;
	}

	let { compositeConfig, catalogId, readonly = false }: Props = $props();

	let loading = $state(false);
	let populated = $state(false);
	let search = $state('');
	let expanded = $state<Record<string, boolean>>({});
	let allDescriptionsEnabled = $state(false);

	// Mocked tools - in real implementation, this would be fetched from the backend
	let tools = $state<ToolMapping[]>([]);

	let filteredTools = $derived(
		tools.filter(
			(tool) =>
				tool.displayName.toLowerCase().includes(search.toLowerCase()) ||
				tool.displayDescription?.toLowerCase().includes(search.toLowerCase())
		)
	);

	let allToolsEnabled = $derived(tools.every((t) => t.enabled));

	function handleToggleDescription(toolId: string, show: boolean) {
		if (allDescriptionsEnabled && !show) {
			allDescriptionsEnabled = false;
			for (const tool of tools) {
				if (toolId !== tool.id) {
					expanded[tool.id] = true;
				}
			}
		}

		expanded[toolId] = show;
		const expandedValues = Object.values(expanded);
		if (expandedValues.length === tools.length && expandedValues.every((v) => v)) {
			allDescriptionsEnabled = true;
		}
	}

	async function handlePopulateTools() {
		if (!catalogId || compositeConfig.componentCatalogEntries.length === 0) return;

		loading = true;
		try {
			// TODO: Backend implementation
			// This would call an API to:
			// 1. Launch temporary instances of each component server
			// 2. Fetch tool lists from each
			// 3. Return aggregated tools with server names

			// Mock data for demonstration
			await new Promise((resolve) => setTimeout(resolve, 2000));

			tools = [
				{
					id: 'github_create_issue',
					serverName: 'GitHub',
					originalName: 'create_issue',
					displayName: 'create_issue',
					originalDescription: 'Create a new issue in a GitHub repository',
					displayDescription: 'Create a new issue in a GitHub repository',
					enabled: true,
					parameters: [
						{
							name: 'repo',
							displayName: 'repo',
							description: 'Repository name in owner/repo format',
							displayDescription: 'Repository name in owner/repo format',
							type: 'string',
							required: true
						},
						{
							name: 'title',
							displayName: 'title',
							description: 'Issue title',
							displayDescription: 'Issue title',
							type: 'string',
							required: true
						}
					]
				},
				{
					id: 'github_list_issues',
					serverName: 'GitHub',
					originalName: 'list_issues',
					displayName: 'list_issues',
					originalDescription: 'List issues in a repository',
					displayDescription: 'List issues in a repository',
					enabled: true
				},
				{
					id: 'slack_send_message',
					serverName: 'Slack',
					originalName: 'send_message',
					displayName: 'send_message',
					displayName: 'send_message',
					originalDescription: 'Send a message to a Slack channel',
					displayDescription: 'Send a message to a Slack channel',
					enabled: false
				}
			];

			populated = true;
		} catch (error) {
			console.error('Failed to populate tools:', error);
		} finally {
			loading = false;
		}
	}

	function toggleAllTools() {
		const newState = !allToolsEnabled;
		tools.forEach((tool) => (tool.enabled = newState));
	}

	function toggleAllDescriptions() {
		allDescriptionsEnabled = !allDescriptionsEnabled;
		for (const tool of tools) {
			expanded[tool.id] = allDescriptionsEnabled;
		}
	}
</script>

<div
	class="dark:bg-surface1 dark:border-surface3 flex flex-col gap-4 rounded-lg border border-transparent bg-white p-4 shadow-sm"
>
	<div class="flex items-center justify-between">
		<h4 class="text-sm font-semibold">Tool Configuration (Optional)</h4>
		{#if !readonly && compositeConfig.componentCatalogEntries.length > 0}
			<button
				class="button-primary text-xs"
				onclick={handlePopulateTools}
				disabled={loading}
			>
				{#if loading}
					<LoaderCircle class="size-4 animate-spin" />
					Populating Tools...
				{:else if populated}
					Refresh Tools
				{:else}
					Auto-Populate Tools
				{/if}
			</button>
		{/if}
	</div>

	{#if compositeConfig.componentCatalogEntries.length === 0}
		<p class="text-sm text-gray-500 dark:text-gray-400">
			Add component MCP servers above to configure tools.
		</p>
	{:else if !populated && !loading}
		<div class="flex flex-col items-center gap-4 py-8 text-center">
			<Wrench class="size-16 text-gray-200 dark:text-gray-700" />
			<div class="flex flex-col gap-2">
				<p class="text-sm font-medium text-gray-600 dark:text-gray-400">
					Tool configuration is optional
				</p>
				<p class="text-xs text-gray-500 dark:text-gray-500">
					By default, all tools from component servers will be available. Click "Auto-Populate Tools"
					to customize which tools are exposed and modify their names/descriptions.
				</p>
			</div>
		</div>
	{:else if loading}
		<div class="flex items-center justify-center py-8">
			<LoaderCircle class="size-6 animate-spin" />
		</div>
	{:else if tools.length > 0}
		<div class="flex flex-col gap-4">
			<div class="flex items-center gap-4">
				<Search
					class="dark:bg-surface2 dark:border-surface3 flex-1 border border-transparent shadow-inner"
					onChange={(val) => (search = val)}
					placeholder="Search tools..."
				/>
				<div class="flex items-center gap-2">
					<Toggle
						checked={allToolsEnabled}
						onToggle={toggleAllTools}
						disabled={readonly}
						label="Enable All"
					/>
				</div>
			</div>

			<div class="flex flex-col gap-2">
				{#each filteredTools as tool (tool.id)}
					<div
						class="dark:bg-surface2 dark:border-surface3 flex flex-col gap-2 rounded-lg border border-gray-200 bg-gray-50 p-3"
					>
						<div class="flex items-start justify-between gap-2">
							<div class="flex min-w-0 flex-1 flex-col gap-1">
								<div class="flex items-center gap-2">
									<span class="dark:bg-surface3 bg-surface2 rounded px-2 py-0.5 text-xs font-medium">
										{tool.serverName}
									</span>
									{#if !readonly}
										<input
											type="text"
											class="text-input-filled flex-1 text-sm font-medium"
											bind:value={tool.displayName}
											placeholder="Tool name"
										/>
									{:else}
										<span class="text-sm font-medium">{tool.displayName}</span>
									{/if}
								</div>
								{#if !readonly}
									<textarea
										class="text-input-filled w-full text-xs"
										bind:value={tool.displayDescription}
										placeholder="Tool description"
										rows="2"
									/>
								{:else}
									<p class="text-xs text-gray-600 dark:text-gray-400">
										{tool.displayDescription}
									</p>
								{/if}
							</div>
							<div class="flex items-center gap-2">
								<Toggle
									checked={tool.enabled}
									onToggle={() => (tool.enabled = !tool.enabled)}
									disabled={readonly}
								/>
								{#if tool.parameters && tool.parameters.length > 0}
									<button
										class="icon-button"
										onclick={() => handleToggleDescription(tool.id, !expanded[tool.id])}
									>
										{#if expanded[tool.id]}
											<ChevronUp class="size-4" />
										{:else}
											<ChevronDown class="size-4" />
										{/if}
									</button>
								{/if}
							</div>
						</div>

						{#if expanded[tool.id] && tool.parameters}
							<div class="flex flex-col gap-2 pt-2" transition:slide={{ duration: 200 }}>
								<div class="border-t border-gray-300 dark:border-gray-600"></div>
								<p class="text-xs font-semibold">Parameters</p>
								{#each tool.parameters as param}
									<div class="flex flex-col gap-1 pl-4">
										<div class="flex items-center gap-2">
											{#if !readonly}
												<input
													type="text"
													class="text-input-filled flex-1 text-xs"
													bind:value={param.displayName}
													placeholder="Parameter name"
												/>
											{:else}
												<span class="text-xs font-medium">{param.displayName}</span>
											{/if}
											<span class="text-xs text-gray-500">
												{param.type}{param.required ? ' (required)' : ''}
											</span>
										</div>
										{#if !readonly}
											<input
												type="text"
												class="text-input-filled w-full text-xs"
												bind:value={param.displayDescription}
												placeholder="Parameter description"
											/>
										{:else}
											<p class="text-xs text-gray-500">{param.displayDescription}</p>
										{/if}
									</div>
								{/each}
							</div>
						{/if}
					</div>
				{/each}
			</div>

			{#if filteredTools.length === 0}
				<p class="py-4 text-center text-sm text-gray-500">No tools match your search.</p>
			{/if}
		</div>
	{/if}

	<p class="text-xs text-gray-500 dark:text-gray-400">
		Configure which tools from your component servers are available in this composite server.
		Customize tool and parameter names/descriptions as needed.
	</p>
</div>
