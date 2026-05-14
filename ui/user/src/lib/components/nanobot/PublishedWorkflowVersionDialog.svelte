<script lang="ts">
	import SearchUsers from '$lib/components/admin/SearchUsers.svelte';
	import { AdminService, NanobotService } from '$lib/services';
	import type { AccessControlRuleSubject, OrgGroup, OrgUser } from '$lib/services/admin/types';
	import type {
		PublishedArtifactVersion,
		PublishedArtifactUpdateRequest
	} from '$lib/services/nanobot/types';
	import { responsive } from '$lib/stores';
	import { formatTimeAgo } from '$lib/time';
	import { getUserDisplayName } from '$lib/utils';
	import MarkdownEditor from './MarkdownEditor.svelte';
	import { hasAllUsersSubject } from './publishedArtifactSubjects';
	import { ChevronLeft, CircleAlert, Plus, Trash2 } from 'lucide-svelte';
	import { fly } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		versions: PublishedArtifactVersion[];
		publishedArtifactId?: string;
		workflowDisplayName?: string;
		onClose: () => void;
		onChangeSubjects: (version: number, subjects: AccessControlRuleSubject[]) => void;
		onUnpublish: () => void;
	}

	let {
		versions,
		workflowDisplayName,
		onClose,
		onChangeSubjects,
		onUnpublish,
		publishedArtifactId
	}: Props = $props();

	let selectedVersionNumber = $state<number | undefined>(undefined);
	let loadingVersion = $state(false);
	let versionContents = $state('');
	let savingSubjects = $state(false);
	let addUserGroupDialog = $state<ReturnType<typeof SearchUsers>>();
	let users = $state<OrgUser[]>([]);
	let groups = $state<OrgGroup[]>([]);

	let userMap = $derived(new Map(users.map((user) => [user.id, user])));
	let groupMap = $derived(new Map(groups.map((group) => [group.id, group])));
	let sortedVersions = $derived([...versions].sort((a, b) => b.version - a.version));
	let selectedVersion = $derived(
		selectedVersionNumber == null
			? undefined
			: sortedVersions.find((version) => version.version === selectedVersionNumber)
	);
	let activeVersion = $derived(selectedVersion ?? sortedVersions[0]);
	let activeSubjects = $derived(activeVersion?.subjects ?? []);

	$effect(() => {
		Promise.all([
			AdminService.listUsers().catch(() => []),
			AdminService.listGroups().catch(() => [])
		]).then(([loadedUsers, loadedGroups]) => {
			users = loadedUsers;
			groups = loadedGroups;
		});
	});

	async function fetchVersionContents(selectedVersion: PublishedArtifactVersion) {
		if (!publishedArtifactId) return;
		selectedVersionNumber = selectedVersion.version;
		loadingVersion = true;
		versionContents = '';
		try {
			versionContents = await NanobotService.getPublishedArtifactVersionContents(
				publishedArtifactId,
				selectedVersion.version
			);
		} catch (error) {
			console.error('Failed to fetch published artifact version contents', error);
		} finally {
			loadingVersion = false;
		}
	}

	async function persistSubjects(nextSubjects: AccessControlRuleSubject[]) {
		if (!publishedArtifactId || !activeVersion) return;

		savingSubjects = true;
		try {
			await NanobotService.updatePublishedArtifact(publishedArtifactId, {
				version: activeVersion.version,
				subjects: nextSubjects
			} satisfies PublishedArtifactUpdateRequest);
			onChangeSubjects(activeVersion.version, nextSubjects);
		} catch (err) {
			console.error('Failed to update published artifact subjects', err);
		} finally {
			savingSubjects = false;
		}
	}

	function getSubjectDisplayName(subject: AccessControlRuleSubject): string {
		if (subject.type === 'selector' && subject.id === '*') {
			return 'All Obot Users';
		}
		if (subject.type === 'group') {
			return groupMap.get(subject.id)?.name ?? subject.id;
		}
		return getUserDisplayName(userMap, subject.id);
	}

	function getSubjectType(subject: AccessControlRuleSubject): string {
		if (subject.type === 'selector') {
			return 'Everyone';
		}
		return subject.type === 'group' ? 'Group' : 'User';
	}
</script>

<dialog class="modal-open modal flex items-center justify-center gap-4">
	{#if responsive.isMobile}
		{#if selectedVersion}
			{@render versionContentsDialog()}
		{:else}
			{@render versionHistoryDialog()}
		{/if}
	{:else}
		{@render versionHistoryDialog()}
		{#if selectedVersion}
			{@render versionContentsDialog()}
		{/if}
	{/if}
</dialog>

{#snippet versionHistoryDialog()}
	<div
		class="modal-box dialog-container flex h-full w-full max-w-full flex-col md:max-h-fit md:max-w-xl"
	>
		<form method="dialog">
			<button class="btn btn-circle btn-ghost btn-sm absolute top-2 right-2" onclick={onClose}
				>✕</button
			>
		</form>
		<div class="flex items-center gap-2">
			<h3 class="text-lg font-semibold">{workflowDisplayName}</h3>
		</div>

		<div class="bg-primary/10 mt-2 flex items-center gap-2 rounded-md px-4 py-2">
			<CircleAlert class="text-primary size-5 flex-shrink-0" />
			<p class="text-base-content mt-1 text-sm font-light">
				{#if !activeVersion}
					Select a version to manage sharing.
				{:else if activeSubjects.length === 0}
					Version {activeVersion.version} is currently only visible to you.
				{:else if hasAllUsersSubject(activeSubjects)}
					This workflow is visible to all Obot users.
				{:else}
					This workflow is visible only to the listed users and groups.
				{/if}
			</p>
		</div>

		<div class="mt-4">
			{@render accessSubjectsPanel()}
		</div>

		{#if versions.length === 0}
			<div class="text-base-content/50 my-4 text-center text-sm">
				<p class="font-medium">No versions found.</p>
			</div>
		{:else}
			<ul class="timeline timeline-snap-icon timeline-compact timeline-vertical mt-4 pr-2">
				{#each sortedVersions as version, index (version.version)}
					<li>
						{#if index > 0}
							<hr />
						{/if}
						<div class="timeline-middle">
							<div
								class={twMerge(
									'badge badge-sm badge-soft w-12',
									activeVersion?.version === version.version ? 'badge-primary' : 'badge-secondary'
								)}
							>
								{parseFloat(version.version.toString()).toFixed(1)}
							</div>
						</div>
						<button
							class="timeline-end hover:bg-base-200 group mt-1 mb-2 w-full rounded-sm px-2 py-1 text-left"
							onclick={() => fetchVersionContents(version)}
						>
							<div class="flex items-center gap-2">
								<div class="flex w-[calc(100%-32px)] flex-col gap-1">
									<p class="text-xs font-light">
										{formatTimeAgo(version.createdAt).relativeTime}
									</p>
									<p class="btn-link group-hover:text-primary line-clamp-1 truncate text-sm">
										{version.description}
									</p>
								</div>
							</div>
						</button>
						{#if index < sortedVersions.length - 1}
							<hr />
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
		<div class="flex grow"></div>

		<div class="modal-action mt-4">
			<button class="btn btn-secondary" onclick={() => onUnpublish()}>
				{versions.length > 1 ? 'Unpublish All' : 'Unpublish'}
			</button>
		</div>
	</div>
{/snippet}

{#snippet accessSubjectsPanel()}
	<div class="mb-2 flex items-center justify-between">
		<h4 class="text-sm font-semibold">
			Access Subjects {#if activeVersion}for v{activeVersion.version}{/if}
		</h4>
		<button
			class="btn btn-ghost btn-sm"
			disabled={!activeVersion}
			onclick={() => addUserGroupDialog?.open()}
		>
			<Plus class="size-4" /> Add User/Group
		</button>
	</div>
	<div class="border-base-300 rounded-md border">
		{#if !activeVersion}
			<p class="text-base-content/60 px-3 py-3 text-sm">Select a version</p>
		{:else if activeSubjects.length === 0}
			<p class="text-base-content/60 px-3 py-3 text-sm">Owner only</p>
		{:else}
			{#each activeSubjects as subject (subject.type + ':' + subject.id)}
				<div
					class="border-base-300 flex items-center justify-between border-b px-3 py-2 last:border-b-0"
				>
					<div class="min-w-0">
						<p class="truncate text-sm font-medium">{getSubjectDisplayName(subject)}</p>
						<p class="text-base-content/60 text-xs">{getSubjectType(subject)}</p>
					</div>
					<button
						class="btn btn-ghost btn-xs btn-square"
						disabled={savingSubjects}
						onclick={() =>
							persistSubjects(
								activeSubjects.filter(
									(current) => !(current.type === subject.type && current.id === subject.id)
								)
							)}
					>
						<Trash2 class="size-4" />
					</button>
				</div>
			{/each}
		{/if}
	</div>
{/snippet}

{#snippet versionContentsDialog()}
	<div
		transition:fly={{ x: 100, duration: 150 }}
		class="modal-box dialog-container h-full max-h-full w-full max-w-full md:max-h-[70dvh] md:max-w-4xl"
	>
		<form method="dialog">
			<button
				class="btn btn-circle btn-ghost btn-sm absolute top-2 right-2"
				onclick={() => (selectedVersionNumber = undefined)}>✕</button
			>
		</form>
		<div class="flex items-center gap-2">
			{#if responsive.isMobile}
				<button
					class="btn btn-circle btn-ghost"
					onclick={() => (selectedVersionNumber = undefined)}
				>
					<ChevronLeft class="size-5" />
				</button>
			{/if}
			<h3 class="text-lg font-semibold">
				{workflowDisplayName} | Version {selectedVersion?.version}
			</h3>
		</div>
		<div class="mt-2 min-h-[200px]">
			{#if responsive.isMobile}
				<div class="mb-4">
					{@render accessSubjectsPanel()}
				</div>
			{/if}
			{#if loadingVersion}
				<div class="flex items-center justify-center gap-2 py-8">
					<span class="loading loading-sm loading-spinner"></span>
					<span>Loading version contents...</span>
				</div>
			{:else if versionContents}
				<div
					class="nanobot border-base-300 bg-base-200 max-h-[60vh] overflow-auto rounded-md border p-2"
				>
					<MarkdownEditor value={versionContents} readonly />
				</div>
			{:else}
				<div class="text-base-content/50 py-8 text-center text-sm">No version contents found.</div>
			{/if}
		</div>
	</div>
{/snippet}

<SearchUsers
	bind:this={addUserGroupDialog}
	filterIds={activeSubjects.map((subject) => subject.id)}
	initialUsers={users}
	initialGroups={groups}
	onAdd={(addedUsers: OrgUser[], addedGroups: OrgGroup[]) => {
		const existingSubjectIds = new Set(activeSubjects.map((subject) => subject.id));
		const nextSubjects = addedGroups.some((entry) => entry.id === '*')
			? [{ type: 'selector' as const, id: '*' }]
			: [
					...activeSubjects.filter(
						(subject) => !(subject.type === 'selector' && subject.id === '*')
					),
					...addedUsers
						.filter((entry) => !existingSubjectIds.has(entry.id))
						.map((entry) => ({ type: 'user' as const, id: entry.id })),
					...addedGroups
						.filter((entry) => !existingSubjectIds.has(entry.id))
						.map((entry) => ({
							type: 'group' as const,
							id: entry.id
						}))
				];
		persistSubjects(nextSubjects);
	}}
/>
