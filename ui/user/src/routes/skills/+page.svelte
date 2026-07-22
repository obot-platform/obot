<script lang="ts">
	import { page } from '$app/state';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import CopyField from '$lib/components/CopyField.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import ObotCliBanner from '$lib/components/ObotCliBanner.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Search from '$lib/components/Search.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { sanitizeFilenameSegment, saveBlob } from '$lib/download';
	import { UserService } from '$lib/services';
	import type { Skill } from '$lib/services/nanobot/types';
	import {
		AiClient,
		COMMON_AI_CLIENTS_MAP,
		MCP_CONNECTION_INVALID_LICENSE_MESSAGE
	} from '$lib/services/user/constants';
	import { formatTimeAgo } from '$lib/time';
	import { setUrlParamAndUpdateUrl } from '$lib/url.js';
	import { TriangleAlert, PencilRuler, Bot, Download } from '@lucide/svelte';
	import { untrack } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	let { data } = $props();
	let query = $derived(page.url.searchParams.get('query') ?? '');

	let skills = $state<Skill[]>(untrack(() => data?.skills ?? []));

	let copyFields = $state<ReturnType<typeof CopyField>[]>([]);
	let installSkillDialog = $state<ReturnType<typeof ResponsiveDialog> | undefined>(undefined);
	let selectedSkillToInstall = $state<Skill | undefined>(undefined);
	let selectedTab = $state<'macos/linux' | 'windows'>('macos/linux');

	let clientUnzipCommands = $derived.by(() => {
		const skillName = sanitizeFilenameSegment(
			selectedSkillToInstall?.name ?? selectedSkillToInstall?.id ?? 'skill'
		);
		const command = (clientSkillsDirectory: string) => {
			if (selectedTab === 'windows') {
				const windowsSkillsDirectory = clientSkillsDirectory.replaceAll('/', '\\');
				return `mkdir "%USERPROFILE%\\${windowsSkillsDirectory}\\${skillName}" 2>NUL & tar -xf "%USERPROFILE%\\Downloads\\${skillName}.zip" -C "%USERPROFILE%\\${windowsSkillsDirectory}\\${skillName}"`;
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
		query
			? skills.filter(
					(d) =>
						d.displayName?.toLowerCase().includes(query.toLowerCase()) ||
						d.name?.toLowerCase().includes(query.toLowerCase()) ||
						d.description?.toLowerCase().includes(query.toLowerCase())
				)
			: skills
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

<Layout classes={{ navbar: 'bg-base-200', container: 'pt-0' }} title="Skills">
	<div class="flex min-h-full flex-col gap-2">
		<ObotCliBanner description="Download the Obot CLI to install skills." />
		<div class="flex min-h-full flex-col">
			<div class="bg-base-200 dark:bg-base-100 sticky top-16 left-0 z-20 w-full py-1">
				<div class="mb-2">
					<Search
						class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
						value={query}
						onChange={updateSearchQuery}
						placeholder="Search skills..."
					/>
				</div>
			</div>

			<div class="dark:bg-base-300 bg-base-100 rounded-t-md shadow-sm">
				{@render skillsView()}
			</div>
		</div>
	</div>
</Layout>

{#snippet skillsView()}
	<div class="flex flex-col gap-2">
		{#if data?.showLicenseError}
			<div class="my-12 flex w-md flex-col items-center gap-4 self-center text-center">
				<TriangleAlert class="size-12 text-warning" />
				<h4 class="text-muted-content text-lg font-semibold">Limited Functionality</h4>
				<p class="text-muted-content text-sm font-light">
					{MCP_CONNECTION_INVALID_LICENSE_MESSAGE}
				</p>
			</div>
		{:else if skills.length > 0}
			<Table
				data={skillsTableData}
				fields={['displayName', 'description', 'created']}
				noDataMessage="No skills found."
				classes={{
					root: 'rounded-none rounded-b-md shadow-none'
				}}
				columnMaxWidths={{ created: 240 }}
				sortable={['displayName', 'created']}
				headers={[
					{
						title: 'Name',
						property: 'displayName'
					}
				]}
				setRowClasses={(d) => {
					if (d.validationError) {
						return 'opacity-50 cursor-default dark:hover:bg-transparent hover:bg-transparent';
					}
					return '';
				}}
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
					{:else if property === 'created'}
						{formatTimeAgo(d.created).relativeTime}
					{:else if property === 'description'}
						<span class="line-clamp-2">{d.description ?? '—'}</span>
					{:else}
						{d[property as keyof typeof d]}
					{/if}
				{/snippet}
				{#snippet actions(d)}
					<div id={`install-skill-btn-container-${d.id}`}>
						<button
							class="btn btn-primary btn-sm"
							id={`install-skill-btn-${d.id}`}
							onclick={() => {
								selectedSkillToInstall = d;
								installSkillDialog?.open();
							}}
						>
							Install
						</button>
					</div>
				{/snippet}
			</Table>
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
{/snippet}

<ResponsiveDialog
	bind:this={installSkillDialog}
	animate="slide"
	id="install-skill-dialog"
	title={selectedSkillToInstall?.displayName}
>
	<div class="w-full @container md:px-0 px-4">
		<div id="download-skill-container">
			<div class="divider mt-0">1. Download {selectedSkillToInstall?.displayName}</div>
			<div class="md:p-0 pb-0 p-4">
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
				>
					macOS/Linux
				</button>
				<button
					role="tab"
					class={twMerge('tab', selectedTab === 'windows' && 'tab-active')}
					onclick={() => (selectedTab = 'windows')}
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

<svelte:head>
	<title>Obot | Skills</title>
</svelte:head>
