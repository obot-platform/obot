<script lang="ts">
	import Toggle from '$lib/components/Toggle.svelte';
	import type { ToolOverride } from '$lib/services';
	import { conflictIssue, effectiveToolName, toolNameIssue } from '$lib/services/user/mcp';
	import Search from '../Search.svelte';
	import ToolNameIssueIcon from '../mcp/ToolNameIssueIcon.svelte';
	import { RefreshCcw } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		tools: ToolOverride[];
		toolPrefix?: string;
		componentId: string;
		readonly?: boolean;
		lockedTools?: Set<string>;
		lockedReason?: string;
		effectiveNameDuplicates?: Set<string>;
		onRefresh?: () => void;
		onToolsChange?: () => void;
	}

	let {
		tools = $bindable(),
		toolPrefix,
		readonly,
		lockedTools,
		lockedReason,
		effectiveNameDuplicates = new Set(),
		onRefresh,
		onToolsChange
	}: Props = $props();

	let search = $state('');

	const unlockedTools = $derived(
		tools.filter((tool) => !tool.removed && !lockedTools?.has(tool.name))
	);

	const allUnlockedToolsEnabled = $derived(
		unlockedTools.length > 0 && unlockedTools.every((tool) => tool.enabled !== false)
	);

	function setUnlockedToolsEnabled(enabled: boolean) {
		tools = tools.map((tool) =>
			tool.removed || lockedTools?.has(tool.name) ? tool : { ...tool, enabled }
		);
		onToolsChange?.();
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
		const available: ToolOverride[] = [];
		const locked: ToolOverride[] = [];
		const removed: ToolOverride[] = [];
		for (const tool of filtered) {
			if (tool.removed) removed.push(tool);
			else if (lockedTools?.has(tool.name)) locked.push(tool);
			else available.push(tool);
		}
		return locked.length > 0 || removed.length > 0
			? [...available, ...locked, ...removed]
			: filtered;
	});
</script>

<div class="flex flex-col gap-2">
	<Search
		class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
		onChange={(val) => (search = val)}
		placeholder="Search tools..."
	/>

	<div class="flex w-full justify-end items-center px-2">
		<div>
			{#if onRefresh}
				<button
					type="button"
					class="btn-sm btn-outline btn not-hover:border-muted-content/50 not-hover:text-muted-content rounded-full hover:btn-primary hover:btn-outline"
					onclick={onRefresh}
				>
					<RefreshCcw class="size-4" /> Refresh tools
				</button>
			{/if}
		</div>
		<div class="divider divider-horizontal mx-2"></div>
		<Toggle
			checked={allUnlockedToolsEnabled}
			disabled={readonly || unlockedTools.length === 0}
			onChange={setUnlockedToolsEnabled}
			label="Enable All Tools"
			labelInline
			disablePortal
			classes={{
				label: 'text-xs gap-2'
			}}
		/>
	</div>

	{#each orderedTools as tool (tool.name)}
		{@const currentName = (tool.overrideName || '').trim() || tool.name}
		{@const currentDescription = (tool.overrideDescription || '').trim() || tool.description}
		{@const name = effectiveToolName(tool.name, tool.overrideName, toolPrefix)}
		{@const conflict =
			tool.enabled !== false ? conflictIssue(name, effectiveNameDuplicates) : undefined}
		{@const unavailable = tool.removed === true}
		{@const locked = !unavailable && (lockedTools?.has(tool.name) ?? false)}

		<div
			class={twMerge(
				'dark:bg-base-300 dark:border-base-400 flex items-start gap-2 rounded border border-transparent bg-white p-2 shadow-sm',
				(locked || unavailable) && 'opacity-50'
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
						{#if unavailable}
							<p class="text-muted-content mt-1 text-[11px] italic">
								This tool is no longer available.
							</p>
						{:else if locked && lockedReason}
							<p class="text-muted-content mt-1 text-[11px] italic">{lockedReason}</p>
						{/if}
					</div>
					<div class="flex shrink-0 items-center gap-2">
						<Toggle
							checked={tool.enabled === true}
							disabled={readonly || locked || unavailable}
							onChange={(checked) => {
								if (unavailable) return;
								tool.enabled = checked;
								onToolsChange?.();
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
