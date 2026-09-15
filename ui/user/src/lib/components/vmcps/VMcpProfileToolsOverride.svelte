<script lang="ts">
	import Toggle from '$lib/components/Toggle.svelte';
	import type { ToolOverride } from '$lib/services';
	import { conflictIssue, effectiveToolName, toolNameIssue } from '$lib/services/user/mcp';
	import Search from '../Search.svelte';
	import ToolNameIssueIcon from '../mcp/ToolNameIssueIcon.svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		tools: ToolOverride[];
		toolPrefix?: string;
		componentId: string;
		readonly?: boolean;
		lockedTools?: Set<string>;
		lockedReason?: string;
		effectiveNameDuplicates?: Set<string>;
	}

	let {
		tools = $bindable(),
		toolPrefix,
		readonly,
		lockedTools,
		lockedReason,
		effectiveNameDuplicates = new Set()
	}: Props = $props();

	let search = $state('');

	const unlockedTools = $derived(tools.filter((tool) => !lockedTools?.has(tool.name)));

	const allUnlockedToolsEnabled = $derived(
		unlockedTools.length > 0 && unlockedTools.every((tool) => tool.enabled !== false)
	);

	function setUnlockedToolsEnabled(enabled: boolean) {
		tools = tools.map((tool) => (lockedTools?.has(tool.name) ? tool : { ...tool, enabled }));
	}

	const orderedTools = $derived.by(() => {
		const query = search.trim().toLowerCase();
		const filtered = query
			? tools.filter(
					(tool) =>
						tool.name.toLowerCase().includes(query) ||
						tool.overrideName?.toLowerCase().includes(query) ||
						tool.description?.toLowerCase().includes(query) ||
						tool.overrideDescription?.toLowerCase().includes(query)
				)
			: tools;
		if (!lockedTools?.size) return filtered;
		const unlocked: ToolOverride[] = [];
		const locked: ToolOverride[] = [];
		for (const tool of filtered) {
			(lockedTools.has(tool.name) ? locked : unlocked).push(tool);
		}
		return locked.length > 0 ? [...unlocked, ...locked] : filtered;
	});
</script>

<div class="flex flex-col gap-2">
	<div class="flex w-full justify-end">
		<Toggle
			checked={allUnlockedToolsEnabled}
			disabled={readonly || unlockedTools.length === 0}
			onChange={setUnlockedToolsEnabled}
			label="Enable All Tools"
			labelInline
			disablePortal
			classes={{
				label: 'text-sm gap-2'
			}}
		/>
	</div>
	<Search
		class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
		onChange={(val) => (search = val)}
		placeholder="Search tools..."
	/>
	{#each orderedTools as tool (tool.name)}
		{@const currentName = (tool.overrideName || '').trim() || tool.name}
		{@const currentDescription = (tool.overrideDescription || '').trim() || tool.description}
		{@const name = effectiveToolName(tool.name, tool.overrideName, toolPrefix)}
		{@const conflict =
			tool.enabled !== false ? conflictIssue(name, effectiveNameDuplicates) : undefined}
		{@const locked = lockedTools?.has(tool.name) ?? false}

		<div
			class={twMerge(
				'dark:bg-base-300 dark:border-base-400 flex items-start gap-2 rounded border border-transparent bg-white p-2 shadow-sm',
				locked && 'opacity-50'
			)}
		>
			<div class="flex min-w-0 grow flex-col gap-2">
				<div class="flex items-start justify-between gap-2">
					<div class="min-w-0 flex-1">
						<div class="flex min-w-0 items-center gap-1.5">
							<div class="min-w-0 flex-1 truncate text-sm font-medium" title={name}>
								{#if toolPrefix}<span class="text-base-content/75">{toolPrefix}</span
									>{/if}{currentName}
							</div>
							{#if tool.enabled !== false}
								<ToolNameIssueIcon issue={conflict ?? toolNameIssue(name)} />
							{/if}
						</div>
						{#if currentDescription}
							<p class="line-clamp-2 text-xs" title={currentDescription}>
								{currentDescription}
							</p>
						{/if}
						{#if locked && lockedReason}
							<p class="text-muted-content mt-1 text-[11px] italic">{lockedReason}</p>
						{/if}
					</div>
					<div class="flex shrink-0 items-center gap-2">
						<Toggle
							checked={tool.enabled === true}
							disabled={readonly || locked}
							onChange={(checked) => {
								tool.enabled = checked;
							}}
							label={tool.enabled ? 'Disable tool' : 'Enable tool'}
							disablePortal
						/>
					</div>
				</div>
			</div>
		</div>
	{/each}
</div>
