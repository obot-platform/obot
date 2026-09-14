<script lang="ts">
	import { page } from '$app/state';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import CopyField from '$lib/components/CopyField.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Search from '$lib/components/Search.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { sanitizeFilenameSegment, saveBlob } from '$lib/download';
	import { UserService } from '$lib/services';
	import type { SkillRepository } from '$lib/services/admin/types';
	import type { Skill } from '$lib/services/nanobot/types';
	import { AiClient, COMMON_AI_CLIENTS_MAP } from '$lib/services/user/constants';
	import { profile } from '$lib/stores';
	import { setUrlParamAndUpdateUrl } from '$lib/url';
	import { openUrl } from '$lib/utils.js';
	import { Bot, Download, GitBranch, PencilRuler, TriangleAlert } from '@lucide/svelte';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		skills: Skill[];
		skillRepositories: SkillRepository[];
		showLicenseError: boolean;
		urlFilters: Record<string, string[]>;
		onFilter: (property: string, values: string[]) => void;
		onClearAllFilters: () => void;
	}

	let {
		skills,
		skillRepositories,
		showLicenseError,
		urlFilters,
		onFilter,
		onClearAllFilters
	}: Props = $props();

	let query = $derived(page.url.searchParams.get('query') ?? '');
	let copyFields = $state<ReturnType<typeof CopyField>[]>([]);
	let installSkillDialog = $state<ReturnType<typeof ResponsiveDialog> | undefined>(undefined);
	let selectedSkillToInstall = $state<Skill | undefined>(undefined);
	let selectedTab = $state<'macos/linux' | 'windows'>('macos/linux');
	let skillRepositoriesMap = $derived(new Map(skillRepositories.map((d) => [d.id, d])));
	let clientUnzipCommands = $derived.by(() => {
		const skillName = sanitizeFilenameSegment(
			selectedSkillToInstall?.name ?? selectedSkillToInstall?.id ?? 'skill'
		);
		const command = (clientSkillsDirectory: string) => {
			if (selectedTab === 'windows') {
				const windowsSkillsDirectory = clientSkillsDirectory.replaceAll('/', '\\');
				return `Expand-Archive -LiteralPath "$env:USERPROFILE\\Downloads\\${skillName}.zip" -DestinationPath "$env:USERPROFILE\\${windowsSkillsDirectory}\\${skillName}" -Force`;
			}

			return `unzip "$HOME/Downloads/${skillName}.zip" -d "$HOME/${clientSkillsDirectory}/${skillName}"`;
		};

		return [
			{
				id: 'cursor',
				label: 'Cursor',
				icon: COMMON_AI_CLIENTS_MAP.get(AiClient.Cursor)?.icon,
				iconDark: COMMON_AI_CLIENTS_MAP.get(AiClient.Cursor)?.iconDark,
				command: command('.cursor/skills')
			},
			{
				id: 'claudeCode',
				label: 'Claude Code',
				icon: COMMON_AI_CLIENTS_MAP.get(AiClient.Claude)?.icon,
				iconDark: COMMON_AI_CLIENTS_MAP.get(AiClient.Claude)?.iconDark,
				command: command('.claude/skills')
			},
			{
				id: 'codex',
				label: 'Codex',
				icon: COMMON_AI_CLIENTS_MAP.get(AiClient.Codex)?.icon,
				iconDark: COMMON_AI_CLIENTS_MAP.get(AiClient.Codex)?.iconDark,
				command: command('.codex/skills')
			},
			{
				id: 'other',
				label: 'Other',
				command: command('.agents/skills')
			}
		];
	});
	let skillsTableData = $derived(
		(query
			? skills.filter(
					(d) =>
						d.name?.toLowerCase().includes(query.toLowerCase()) ||
						d.description?.toLowerCase().includes(query.toLowerCase())
				)
			: skills
		).map((d) => ({
			...d,
			repository: d.repoID ? (skillRepositoriesMap.get(d.repoID)?.displayName ?? '') : ''
		}))
	);

	function updateSearchQuery(value: string) {
		setUrlParamAndUpdateUrl(page.url, 'query', value);
	}

	async function handleDownloadSkill(skill?: Skill) {
		if (!skill) return;
		try {
			const blob = await UserService.downloadSkill(skill.id);
			const filename = `${sanitizeFilenameSegment(skill.name ?? skill.id)}.zip`;
			saveBlob(blob, filename);
		} catch (err) {
			console.error('Failed to download skill', err);
		}
	}
</script>

<div class="flex min-h-full flex-col" in:fade={{ duration: PAGE_TRANSITION_DURATION }}>
	<div class="bg-base-200 dark:bg-base-100 sticky top-0 z-20 w-full">
		<div class="mb-2">
			<Search
				class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
				value={query}
				onChange={updateSearchQuery}
				placeholder="Search skills..."
			/>
		</div>
	</div>

	{#if skills.length > 0}
		<Table
			data={skillsTableData}
			fields={profile.current.hasAdminAccess?.()
				? ['displayName', 'description', 'repository']
				: ['displayName', 'description']}
			noDataMessage="No skills found."
			classes={{
				root: 'rounded-md shadow-sm'
			}}
			sortable={profile.current.hasAdminAccess?.()
				? ['displayName', 'repository']
				: ['displayName']}
			filterable={profile.current.hasAdminAccess?.() ? ['repository'] : []}
			headers={[
				{
					title: 'Name',
					property: 'displayName'
				}
			]}
			onClickRow={(d, isCtrlClick) => {
				if (d.valid) {
					openUrl(`/skills/${d.id}`, isCtrlClick);
				}
			}}
			setRowClasses={(d) => {
				if (d.validationError) {
					return 'opacity-50 cursor-default dark:hover:bg-transparent hover:bg-transparent';
				}
				return '';
			}}
			filters={urlFilters}
			{onFilter}
			{onClearAllFilters}
		>
			{#snippet onRenderColumn(property, d)}
				{#if property === 'displayName'}
					<span class="flex items-center gap-2">
						{d.displayName}
						{#if d.validationError}
							<div use:tooltip={{ text: d.validationError }}>
								<TriangleAlert class="size-3 text-warning" />
							</div>
						{/if}
					</span>
				{:else if property === 'repository'}
					<span class="block min-w-0 truncate">{d.repository}</span>
				{:else if property === 'description'}
					<span class="line-clamp-2 text-sm">{d.description ?? '—'}</span>
				{:else}
					{d[property as keyof typeof d]}
				{/if}
			{/snippet}
			{#snippet actions(d)}
				<div class="flex items-center gap-1">
					{#if d.valid}
						<div id={`install-skill-btn-container-${d.id}`}>
							<button
								class="btn btn-primary btn-sm"
								id={`install-skill-btn-${d.id}`}
								onclick={(e) => {
									e.stopPropagation();
									selectedSkillToInstall = d;
									installSkillDialog?.open();
								}}
							>
								Install
							</button>
						</div>
					{/if}
					<a
						class="btn btn-square btn-ghost hover:text-blue-500 btn-sm tooltip tooltip-left"
						href={`${d.repoURL}/tree/${d.repoRef || d.commitSHA || 'main'}/${d.relativePath}`}
						rel="external noopener noreferrer"
						target="_blank"
						onclick={(e) => e.stopPropagation()}
						data-tip="View Source on Git"
					>
						<GitBranch class="size-4" />
					</a>
				</div>
			{/snippet}
		</Table>
	{:else if showLicenseError}
		<div class="my-12 flex w-md flex-col items-center gap-4 self-center text-center">
			<TriangleAlert class="size-12 text-warning" />
			<h4 class="text-muted-content text-lg font-semibold">License Error</h4>
			<p class="text-muted-content text-sm font-light">
				An issue occurred with fetching skills due to licensing. Please resolve outstanding
				licensing issues or contact support at
				<a href="mailto:info@obot.ai" class="text-link">info@obot.ai</a>.
			</p>
		</div>
	{:else}
		<div class="my-12 flex w-md flex-col items-center gap-4 self-center text-center">
			<PencilRuler class="text-base-content/80 size-24" />
			<h4 class="text-muted-content text-lg font-semibold">No current skills.</h4>
			<p class="text-muted-content text-sm font-light">
				Once a Git Source URL has been added, the skills <br />
				discovered will be viewable from here.
			</p>
		</div>
	{/if}
</div>

<ResponsiveDialog
	bind:this={installSkillDialog}
	animate="slide"
	id="install-skill-dialog"
	title={selectedSkillToInstall?.displayName}
>
	<div id="install-skill-dialog-content" class="w-full @container md:px-0 px-4">
		<div id="download-skill-container">
			<div class="divider md:mt-0">1. Download {selectedSkillToInstall?.displayName}</div>
			<div class="md:p-0 md:pb-0 p-4">
				<button
					class="btn btn-primary btn-sm w-full"
					onclick={() => handleDownloadSkill(selectedSkillToInstall)}
				>
					<Download class="size-4" /> Download
				</button>
			</div>
		</div>
		<div class="divider">2. Unzip via CLI</div>
		<div class="relative">
			<p class="absolute top-1/2 -translate-y-1/2 left-2 text-xs font-semibold">Choose your OS:</p>
			<div
				id="install-skill-os-selector"
				role="tablist"
				class="tabs tabs-box tabs-sm flex items-center justify-end mb-1"
			>
				<button
					role="tab"
					class={twMerge('tab', selectedTab === 'macos/linux' && 'tab-active')}
					onclick={() => (selectedTab = 'macos/linux')}
					aria-selected={selectedTab === 'macos/linux'}
				>
					macOS/Linux
				</button>
				<button
					role="tab"
					class={twMerge('tab', selectedTab === 'windows' && 'tab-active')}
					onclick={() => (selectedTab = 'windows')}
					aria-selected={selectedTab === 'windows'}
				>
					Windows
				</button>
			</div>
		</div>
		<div id="unzip-skill-commands-container" class="flex gap-2 flex-col">
			{#each clientUnzipCommands as client, index (client.id)}
				<div id={`unzip-skill-command-${client.id}-container`}>
					<CopyField
						value={client.command}
						id={`command-${client.id}`}
						classes={{
							inputLabel: 'bg-base-100 dark:bg-base-300',
							input: 'font-mono'
						}}
						bind:this={copyFields[index]}
					>
						{#snippet preContent()}
							<span class="label shrink-0 w-38 mr-0 text-base-content">
								{#if client.icon || client.iconDark}
									<img
										src={client.iconDark ?? client.icon}
										alt={`${client.label} branding icon`}
										class="size-4 dark:block hidden"
									/>
									<img
										src={client?.icon}
										alt={`${client.label} branding icon`}
										class="size-4 block dark:hidden"
									/>
								{:else}
									<Bot class="size-4" />
								{/if}
								{client.label}
							</span>
						{/snippet}
					</CopyField>
				</div>
			{/each}
		</div>
	</div>
</ResponsiveDialog>
