<script module lang="ts">
	import type {
		AccessControlRuleSubject,
		OrgGroup,
		ToolOverride,
		VMCPProfile,
		VMCPToolSet
	} from '$lib/services';

	type ProfileResource = { id: string; toolOverrides: ToolOverride[] };
	type ProfileManifest = {
		name: string;
		users: AccessControlRuleSubject[];
		resources: ProfileResource[];
	};
	type Profile = ProfileManifest & { id: string };
</script>

<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import Select from '$lib/components/Select.svelte';
	import VMcpProfileToolsOverride from '$lib/components/vmcps/VMcpProfileToolsOverride.svelte';
	import type { VMcpToolFlow } from '$lib/runes/vmcps/vmcpToolFlow.svelte';
	import { UserService, type OrgUser, type VMCP, type VMCPComponent } from '$lib/services';
	import { compositeEffectiveToolNames, duplicateToolNames } from '$lib/services/user/mcp';
	import { vmcpManifest } from '$lib/services/vmcps/utils';
	import { success } from '$lib/stores/success';
	import { getUserRoleLabel } from '$lib/utils';
	import IconButton from '../primitives/IconButton.svelte';
	import McpServerIcon from './McpServerIcon.svelte';
	import {
		ArrowLeft,
		ChevronDown,
		ChevronUp,
		Plus,
		Server,
		Split,
		Trash2,
		UsersRound,
		X
	} from '@lucide/svelte';
	import { fly, slide } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		vmcp?: VMCP;
		toolFlow: VMcpToolFlow;
		onUpdated?: (vmcp: VMCP) => void;
	}

	let { vmcp, toolFlow, onUpdated }: Props = $props();
	let profiles = $state<Profile[]>([]);
	let draft = $state<ProfileManifest>();
	let editingId = $state<string>();
	let saving = $state(false);
	let error = $state('');
	let directoryUsers = $state<OrgUser[]>([]);
	let directoryGroups = $state<OrgGroup[]>([]);
	let subjectQuery = $state('');
	let subjectSelection = $state<string | number>();
	let expanded = $state<Record<string, boolean>>({});
	let loadedVMcpId = $state<string>();
	let confirmDeleteProfile = $state<{ id: string; name: string }>();

	const EVERYONE_GROUP: OrgGroup = { id: '*', name: 'All Obot Users' };
	const GROUP_PAGE_SIZE = 50;

	const nameError = $derived(
		error === 'Enter a profile name.' || error === 'A profile with this name already exists.'
	);
	const subjectsError = $derived(error === 'Assign at least one person or group.');
	const componentServers = $derived(vmcp?.components ?? []);
	const assignedSubjectIds = $derived(new Set(draft?.users.map((subject) => subject.id) ?? []));
	const subjectOptions = $derived.by(() => {
		const query = subjectQuery.trim().toLowerCase();
		const everyoneMatches = !query || EVERYONE_GROUP.name.toLowerCase().includes(query);
		const users = query
			? directoryUsers.filter(
					(user) =>
						(user.displayName ?? '').toLowerCase().includes(query) ||
						(user.email ?? '').toLowerCase().includes(query) ||
						(user.username ?? '').toLowerCase().includes(query)
				)
			: directoryUsers;
		const groups = everyoneMatches ? [EVERYONE_GROUP, ...directoryGroups] : directoryGroups;
		return [...groups, ...users]
			.filter((entry) => !assignedSubjectIds.has(entry.id))
			.map((entry) => ({
				id: entry.id,
				label: directoryName(entry),
				subject: toSubject(entry)
			}));
	});

	let groupsRequest: AbortController | undefined;

	$effect(() => {
		if (!draft) return;
		void loadUsers();
	});

	$effect(() => {
		if (!draft) return;
		const query = subjectQuery;
		const timer = setTimeout(() => loadGroups(query), query ? 500 : 0);
		return () => {
			clearTimeout(timer);
			groupsRequest?.abort();
		};
	});

	$effect(() => {
		if (!vmcp) return;
		const switched = loadedVMcpId !== vmcp.id;
		loadedVMcpId = vmcp.id;
		if (!switched) return;
		profiles = (vmcp.profiles ?? []).map(profileFromManifest);
		draft = undefined;
		editingId = undefined;
		error = '';
		expanded = {};
	});

	$effect(() => {
		if (!profiles.some((profile) => profile.users.some((subject) => subject.type !== 'selector')))
			return;
		void loadUsers();
		void loadGroups('');
	});

	function componentId(component: VMCPComponent) {
		return component.id || component.mcpServerCatalogEntryID || '';
	}

	function componentName(component: VMCPComponent) {
		return (
			component.name ||
			component.catalogEntry?.manifest?.name ||
			componentId(component) ||
			'Unknown'
		);
	}

	function initialTools(component: VMCPComponent): ToolOverride[] {
		if (component.toolOverrides?.length) {
			return component.toolOverrides.map((tool) => ({ ...tool }));
		}
		return (component.catalogEntry?.manifest?.toolPreview ?? []).map((tool) => ({
			name: tool.name,
			description: tool.description,
			enabled: tool.enabled !== false
		}));
	}

	function cloneProfile<T extends ProfileManifest>(profile: T): T {
		return {
			...profile,
			users: profile.users.map((subject) => ({ ...subject })),
			resources: profile.resources.map((resource) => ({
				...resource,
				toolOverrides: resource.toolOverrides.map((tool) => ({ ...tool }))
			}))
		};
	}

	function profileFromManifest(manifest: VMCPProfile): Profile {
		const granted = manifest.allowAllTools ? undefined : (manifest.allowedTools ?? {});
		return {
			id: crypto.randomUUID(),
			name: manifest.name,
			users: (manifest.subjects ?? []).map((subject) => ({ ...subject })),
			resources: componentServers
				.map((component) => {
					const id = componentId(component);
					const tools = initialTools(component);
					const names = granted?.[id];
					return {
						id,
						toolOverrides: clampToComponent(
							!granted || names?.includes('*')
								? tools
								: tools.map((tool) => ({ ...tool, enabled: (names ?? []).includes(tool.name) })),
							id
						)
					};
				})
				.filter((resource) => resource.id)
		};
	}

	function profileToManifest(profile: Profile): VMCPProfile {
		const allowedTools: VMCPToolSet = {};
		for (const resource of profile.resources) {
			allowedTools[resource.id] = grantedToolNames(resource);
		}
		const allowAllTools =
			profile.resources.length > 0 &&
			Object.values(allowedTools).every((names) => names.length === 1 && names[0] === '*');
		return {
			name: profile.name,
			subjects: profile.users.map((subject) => ({ ...subject })),
			allowAllTools,
			...(allowAllTools ? {} : { allowedTools })
		};
	}

	function grantedToolNames(resource: ProfileResource) {
		if (!resource.toolOverrides.some((tool) => tool.enabled === false)) return ['*'];
		return resource.toolOverrides.filter((tool) => tool.enabled !== false).map((tool) => tool.name);
	}

	function grantableToolNames(id: string) {
		const overrides = componentServers.find(
			(component) => componentId(component) === id
		)?.toolOverrides;
		if (!overrides?.length) return undefined;
		return new Set(overrides.filter((tool) => tool.enabled).map((tool) => tool.name));
	}

	function clampToComponent(tools: ToolOverride[], id: string) {
		const grantable = grantableToolNames(id);
		if (!grantable) return tools;
		return tools.map((tool) =>
			grantable.has(tool.name) ? tool : { ...tool, enabled: false as const }
		);
	}

	function lockedToolNames(resource: ProfileResource) {
		const grantable = grantableToolNames(resource.id);
		if (!grantable) return undefined;
		return new Set(
			resource.toolOverrides.filter((tool) => !grantable.has(tool.name)).map((tool) => tool.name)
		);
	}

	function disableLockedTools(resource: ProfileResource) {
		const locked = lockedToolNames(resource);
		if (!locked?.size) return resource.toolOverrides;
		let changed = false;
		const next = resource.toolOverrides.map((tool) => {
			if (!locked.has(tool.name) || tool.enabled === false) return tool;
			changed = true;
			return { ...tool, enabled: false as const };
		});
		return changed ? next : resource.toolOverrides;
	}

	$effect(() => {
		if (!draft) return;
		for (const resource of draft.resources) {
			resource.toolOverrides = disableLockedTools(resource);
		}
	});

	function createProfile() {
		editingId = undefined;
		error = '';
		expanded = {};
		draft = {
			name: '',
			users: [],
			resources: componentServers
				.map((component) => {
					const id = componentId(component);
					return {
						id,
						toolOverrides: clampToComponent(initialTools(component), id)
					};
				})
				.filter((resource) => resource.id)
		};
	}

	function editProfile(profile: Profile) {
		editingId = profile.id;
		error = '';
		expanded = {};
		draft = cloneProfile(profile);
	}

	function cancelEditing() {
		draft = undefined;
		editingId = undefined;
		error = '';
		expanded = {};
	}

	export function leaveEditor() {
		if (!draft) return false;
		cancelEditing();
		return true;
	}

	async function saveProfile() {
		if (!draft || !vmcp || saving) return;
		const name = draft.name.trim();
		if (!name) {
			error = 'Enter a profile name.';
			return;
		}
		if (profiles.some((profile) => profile.id !== editingId && profile.name.trim() === name)) {
			error = 'A profile with this name already exists.';
			return;
		}
		if (draft.users.length === 0) {
			error = 'Assign at least one person or group.';
			return;
		}

		const profile: Profile = {
			id: editingId ?? crypto.randomUUID(),
			...cloneProfile({ ...draft, name })
		};
		const next = editingId
			? profiles.map((candidate) => (candidate.id === editingId ? profile : candidate))
			: [...profiles, profile];

		if (!(await persistProfiles(next, `${name} saved on ${vmcp.displayName}.`))) {
			error = 'Failed to save this profile.';
			return;
		}
		cancelEditing();
	}

	function promptDelete(id: string, name: string) {
		confirmDeleteProfile = { id, name };
	}

	async function deleteProfile(id: string) {
		if (!vmcp || saving) return false;
		const removed = profiles.find((profile) => profile.id === id);
		if (!removed) return false;

		const next = profiles.filter((profile) => profile.id !== id);
		const saved = await persistProfiles(next, `${removed.name} removed from ${vmcp.displayName}.`);
		if (saved && editingId === id) cancelEditing();
		if (saved) confirmDeleteProfile = undefined;
		return saved;
	}

	async function persistProfiles(next: Profile[], message: string) {
		if (!vmcp) return false;
		saving = true;
		try {
			const latest = await UserService.getVMCP(vmcp.id);
			const updated = await UserService.updateVMCP(latest.id, {
				...vmcpManifest(latest),
				profiles: next.map(profileToManifest)
			});
			profiles = next;
			success.add(message);
			onUpdated?.(updated);
			return true;
		} catch {
			return false;
		} finally {
			saving = false;
		}
	}

	function isDirectoryGroup(entry: OrgUser | OrgGroup): entry is OrgGroup {
		return 'name' in entry;
	}

	function directoryName(entry: OrgUser | OrgGroup) {
		return isDirectoryGroup(entry)
			? entry.name
			: (entry.displayName ?? entry.email ?? entry.username ?? entry.id);
	}

	function toSubject(entry: OrgUser | OrgGroup): AccessControlRuleSubject {
		if (!isDirectoryGroup(entry)) return { type: 'user', id: entry.id };
		return { type: entry.id === EVERYONE_GROUP.id ? 'selector' : 'group', id: entry.id };
	}

	function subjectDisplay(subject: AccessControlRuleSubject) {
		if (subject.type === 'selector') {
			const name = subject.id === EVERYONE_GROUP.id ? EVERYONE_GROUP.name : subject.id;
			return { name, group: true, iconURL: undefined, role: undefined };
		}
		if (subject.type === 'group') {
			const group = directoryGroups.find((candidate) => candidate.id === subject.id);
			return { name: group?.name ?? subject.id, group: true, iconURL: undefined, role: undefined };
		}
		const user = directoryUsers.find((candidate) => candidate.id === subject.id);
		return {
			name: user ? directoryName(user) : subject.id,
			group: false,
			iconURL: user?.iconURL,
			role: user?.effectiveRole
		};
	}

	function userInitials(source: string) {
		if (!source) return '?';
		const local = source.includes('@') ? source.split('@')[0] : source;
		const parts = local.split(/[.\-\s]/).filter(Boolean);
		if (parts.length === 0) return '?';
		let initials = parts[0].charAt(0).toUpperCase();
		if (parts.length > 1) {
			initials += parts[parts.length - 1].charAt(0).toUpperCase();
		}
		return initials;
	}

	function addSubject(subject: AccessControlRuleSubject) {
		subjectSelection = undefined;
		if (!draft) return;
		if (draft.users.some((candidate) => candidate.id === subject.id)) return;
		draft.users = [...draft.users, subject];
		if (subjectsError) error = '';
	}

	function removeSubject(id: string) {
		if (draft) draft.users = draft.users.filter((subject) => subject.id !== id);
	}

	async function loadUsers() {
		if (directoryUsers.length > 0) return;
		try {
			directoryUsers = await UserService.listUsers();
		} catch (err) {
			console.error('Failed to load users:', err);
		}
	}

	async function loadGroups(query: string) {
		groupsRequest?.abort();
		const controller = new AbortController();
		groupsRequest = controller;
		try {
			const page = await UserService.listGroups({
				query: query.trim() || undefined,
				limit: GROUP_PAGE_SIZE,
				signal: controller.signal
			});
			if (controller.signal.aborted) return;
			directoryGroups = [...page.items].sort((a, b) => a.name.localeCompare(b.name));
		} catch (err) {
			if (controller.signal.aborted) return;
			console.error('Failed to load groups:', err);
			directoryGroups = [];
		}
	}

	function resourceFor(id: string) {
		return draft?.resources.find((resource) => resource.id === id);
	}

	const effectiveNameDuplicates = $derived(
		duplicateToolNames(
			compositeEffectiveToolNames(
				componentServers.map((component) => ({
					toolPrefix: component.toolPrefix,
					toolOverrides: resourceFor(componentId(component))?.toolOverrides
				}))
			)
		)
	);

	function modifiableTools(resource: ProfileResource) {
		const locked = lockedToolNames(resource);
		if (!locked) return resource.toolOverrides;
		return resource.toolOverrides.filter((tool) => !locked.has(tool.name));
	}

	function enabledToolCount(resource: ProfileResource) {
		return modifiableTools(resource).filter((tool) => tool.enabled !== false).length;
	}

	function applyComponentToolOverrides(id: string, toolOverrides: ToolOverride[]) {
		const target = componentServers.find((candidate) => componentId(candidate) === id);
		if (target) target.toolOverrides = toolOverrides;
	}

	async function persistComponentTools(id: string, toolOverrides: ToolOverride[]) {
		if (!vmcp) return;
		saving = true;
		try {
			const latest = await UserService.getVMCP(vmcp.id);
			const updated = await UserService.updateVMCP(latest.id, {
				...vmcpManifest(latest),
				components: (latest.components ?? []).map((component) =>
					componentId(component) === id ? { ...component, toolOverrides } : component
				)
			});
			onUpdated?.(updated);
		} catch {
			error = 'Failed to update tools for this server.';
		} finally {
			saving = false;
		}
	}

	function getDisplayListText(names: string[]) {
		if (names.length <= 1) return names[0] ?? '';
		const rest = names.slice(0, names.length > 5 ? 4 : -1);
		const last = names.length > 5 ? `${names.length - 4} others` : names.at(-1);
		return `${rest.join(', ')} and ${last}`;
	}

	function profileUsersDisplayText(profile: Profile) {
		return getDisplayListText(profile.users.map((subject) => subjectDisplay(subject).name));
	}

	function enabledToolNames(tools: ToolOverride[]) {
		return new Set(tools.filter((tool) => tool.enabled !== false).map((tool) => tool.name));
	}

	function profileResourcePreviews(profile: Profile) {
		const summaries = profile.resources
			.map((resource) => {
				const component = componentServers.find(
					(candidate) => componentId(candidate) === resource.id
				);
				const enabled = enabledToolCount(resource);
				const total = modifiableTools(resource).length;
				const componentEnabled = enabledToolNames(
					component
						? modifiableTools({
								id: resource.id,
								toolOverrides: clampToComponent(initialTools(component), resource.id)
							})
						: []
				);
				const profileEnabled = enabledToolNames(modifiableTools(resource));
				const changed =
					[...profileEnabled].some((name) => !componentEnabled.has(name)) ||
					[...componentEnabled].some((name) => !profileEnabled.has(name));
				return {
					id: resource.id,
					name: component ? componentName(component) : resource.id,
					icon: component?.catalogEntry?.manifest?.icon,
					enabled,
					total,
					changed
				};
			})
			.sort((a, b) => Number(b.changed) - Number(a.changed));

		return {
			items: summaries.slice(0, 4),
			more: Math.max(summaries.length - 4, 0)
		};
	}

	function refineTools(event: MouseEvent, component: VMCPComponent) {
		event.preventDefault();
		event.stopPropagation();
		if (!vmcp) return;
		toolFlow.collectComponentTools(component, vmcp, (config) => {
			if (!draft) return;
			const id = componentId(component);
			const policyToolOverrides = (config.toolOverrides ?? []).map((tool) => ({ ...tool }));
			const componentToolOverrides = policyToolOverrides.map((tool) => ({
				...tool,
				enabled: true
			}));
			applyComponentToolOverrides(id, componentToolOverrides);
			draft.resources = draft.resources.map((resource) =>
				resource.id === id
					? {
							...resource,
							toolOverrides: clampToComponent(policyToolOverrides, id)
						}
					: resource
			);
			expanded[id] = true;
			void persistComponentTools(id, componentToolOverrides);
		});
	}
</script>

<div class="p-3 pt-0 @container">
	{#if draft}
		<div class="mx-auto w-full max-w-4xl">
			{@render editCreate()}
		</div>
	{:else}
		<p class="text-muted-content text-sm font-light mt-2 mb-4">
			Profiles let you control which tools are available to different users, groups, and agents.
			Define a set of tools and assign identities to the profile to provide tailored access through
			the same VMCP endpoint.
		</p>
		{#if vmcp}
			{@render actions()}
			{@render list()}
		{:else}
			<div class="flex flex-col items-center justify-center text-center">
				<div
					class="bg-primary/10 text-primary mb-4 flex size-9 items-center justify-center rounded-md"
				>
					<UsersRound class="size-4" />
				</div>
				<h2 class="font-semibold">No profiles yet</h2>
				<p class="text-muted-content mt-1 max-w-sm text-xs">
					In order to create profiles, you must first create a vMCP.
				</p>
			</div>
		{/if}
	{/if}
</div>

<Confirm
	show={Boolean(confirmDeleteProfile)}
	onsuccess={() => {
		if (!confirmDeleteProfile) return;
		void deleteProfile(confirmDeleteProfile.id);
	}}
	oncancel={() => (confirmDeleteProfile = undefined)}
	msg=""
	loading={saving}
	title="Confirm Delete"
>
	{#snippet note()}
		Are you sure you want to delete "<b>{confirmDeleteProfile?.name ?? 'this profile'}</b>"? This
		cannot be undone.
	{/snippet}
</Confirm>

{#snippet actions()}
	{#if !draft && profiles.length > 0}
		<div class="absolute top-3 right-3 z-50">
			<button
				class="btn btn-primary"
				onclick={() => {
					createProfile();
				}}
			>
				<Plus class="size-4" /> Create profile
			</button>
		</div>
	{/if}
{/snippet}

{#snippet editCreate()}
	{#if draft}
		<form
			class="flex flex-col gap-3 border-base-300 bg-base-100 dark:bg-base-300 rounded-xl border p-5 shadow-sm"
			onsubmit={(event) => {
				event.preventDefault();
				void saveProfile();
			}}
		>
			<header class="flex items-start gap-3">
				<IconButton tooltip={{ text: 'Back to profiles' }} onclick={cancelEditing}>
					<ArrowLeft class="size-4" />
				</IconButton>
				<div class="min-w-0 grow">
					<h2 class="text-md font-semibold">{editingId ? 'Edit profile' : 'Create profile'}</h2>
					<p class="text-muted-content text-sm">
						A profile defines a set of tools for this vMCP that a user, agent, or group has access
						to.
					</p>
				</div>
				{#if editingId}
					<IconButton
						variant="danger"
						disabled={saving}
						tooltip={{ text: `Delete ${draft?.name ?? 'profile'}`, placement: 'bottom' }}
						onclick={() => {
							if (!editingId || !draft) return;
							promptDelete(editingId, draft.name);
						}}
					>
						<Trash2 class="size-4" />
					</IconButton>
				{/if}
			</header>

			<div class="divider my-0"></div>

			<section>
				<label class="flex flex-col gap-0.5" for="profile-name">
					<span class={twMerge('font-light text-sm', nameError && 'text-error')}>Name</span>
					<input
						id="profile-name"
						class={twMerge(
							'input-text-filled text-sm',
							nameError && 'border-error bg-error/20 ring-error ring-1 focus:ring-1'
						)}
						placeholder="ex. Marketing, Engineering, etc."
						autocomplete="off"
						aria-invalid={nameError || undefined}
						bind:value={draft.name}
						oninput={() => (error = '')}
					/>
				</label>
			</section>

			<section>
				<div class="divider text-sm font-semibold my-3">MCP Servers</div>
				<div class="mb-4">
					<p class="text-muted-content text-sm font-light">
						Further modify the tools available for each MCP server in this profile below.
					</p>
				</div>
				{#if componentServers.length === 0}
					<div class="text-muted-content rounded-lg p-5 text-center text-sm">
						No MCP servers available.
					</div>
				{:else}
					<div class="flex flex-col gap-1">
						{#each componentServers as component (componentId(component))}
							{@const id = componentId(component)}
							{@const resource = resourceFor(id)}
							<div class="border-base-300 dark:border-base-400 overflow-hidden rounded-lg border">
								{#snippet componentIdentity()}
									{#if component.catalogEntry?.manifest?.icon}
										<img src={component.catalogEntry.manifest.icon} alt="" class="size-5" />
									{:else}
										<div class="icon">
											<Server class="size-5" />
										</div>
									{/if}
									<span class="grow font-medium text-sm">{componentName(component)}</span>
								{/snippet}
								{#if resource && resource.toolOverrides.length > 0}
									<button
										type="button"
										class="hover:bg-base-200 dark:hover:bg-base-200/60 flex w-full items-center gap-3 py-1 pl-3 pr-1 text-left"
										aria-expanded={Boolean(expanded[id])}
										aria-label={expanded[id] ? 'Collapse' : 'Expand'}
										onclick={() => (expanded[id] = !expanded[id])}
									>
										{@render componentIdentity()}
										<span class="text-muted-content text-xs">
											{enabledToolCount(resource)} of {modifiableTools(resource).length} tools
										</span>
										<span
											class="text-muted-content flex size-8 shrink-0 items-center justify-center"
											aria-hidden="true"
										>
											{#if expanded[id]}
												<ChevronUp class="size-4" />
											{:else}
												<ChevronDown class="size-4" />
											{/if}
										</span>
									</button>
								{:else}
									<div class="flex items-center gap-3 py-1 pl-3 pr-1">
										{@render componentIdentity()}
										{#if resource}
											<IconButton
												class="size-8"
												type="button"
												tooltip={{ text: 'Refine tools' }}
												onclick={(event) => refineTools(event, component)}
											>
												<Split class="size-4" />
											</IconButton>
										{/if}
									</div>
								{/if}
								{#if resource && resource.toolOverrides.length > 0 && expanded[id]}
									<div
										in:slide={{ axis: 'y', duration: 150 }}
										class="border-base-300 bg-base-200/35 dark:bg-base-200 flex flex-col border-t p-2"
									>
										<VMcpProfileToolsOverride
											bind:tools={resource.toolOverrides}
											toolPrefix={component.toolPrefix}
											componentId={id}
											lockedTools={lockedToolNames(resource)}
											lockedReason="Disabled on this vMCP."
											{effectiveNameDuplicates}
										/>
									</div>
								{/if}
							</div>
						{/each}
					</div>
				{/if}
			</section>

			<section>
				<div class="divider text-sm font-semibold my-3">Identities</div>
				<div class="mb-4">
					<p class="text-muted-content text-sm font-light">
						Grant the following users, agents, and groups access to this profile.
					</p>
				</div>
				<Select
					id="profile-subjects"
					options={subjectOptions}
					bind:query={subjectQuery}
					bind:selected={subjectSelection}
					searchInDropdown
					placeholder="Add identities..."
					searchPlaceholder="Search users or groups..."
					class="bg-base-200 shadow-inner!"
					classes={{ root: 'w-full' }}
					invalid={subjectsError}
					onSelect={(option) => addSubject(option.subject)}
				/>
				{#if draft.users.length === 0}
					<div class="text-muted-content text-center pb-4 pt-3 text-xs italic font-light">
						No people or groups assigned.
					</div>
				{:else}
					<div class="flex flex-col mt-2">
						{#each draft.users as subject, index (subject.id)}
							{@const display = subjectDisplay(subject)}
							<div
								class="p-2 flex justify-between items-center"
								out:fly={{ x: 100, duration: 100 }}
							>
								<div class="flex items-center gap-2">
									{#if display.group}
										<div
											class="bg-base-300 dark:bg-base-200 flex size-8 items-center justify-center rounded-full text-[8px] font-medium text-white"
										>
											<UsersRound class="size-4" />
										</div>
									{:else if display.iconURL}
										<img
											src={display.iconURL}
											alt=""
											class="size-4 rounded-full object-cover"
											referrerpolicy="no-referrer"
										/>
									{:else}
										<div
											class="bg-base-300 dark:bg-base-200 flex size-8 items-center justify-center rounded-full text-sm font-medium text-white"
										>
											{userInitials(display.name)}
										</div>
									{/if}
									<div class="flex flex-col">
										<span class="text-sm font-light">{display.name}</span>
										<span class="text-muted-content text-xs">
											{display.group
												? 'Group'
												: display.role
													? getUserRoleLabel(display.role)
													: 'User'}
										</span>
									</div>
								</div>

								<IconButton
									class="size-8"
									tooltip={{ text: `Remove ${display.name}` }}
									onclick={() => removeSubject(subject.id)}
									variant="danger"
								>
									<X class="size-4" />
								</IconButton>
							</div>
							{#if index < draft.users.length - 1}
								<div class="divider my-0 after:h-px before:h-px px-2"></div>
							{/if}
						{/each}
					</div>
				{/if}
			</section>
			<p class="text-error text-center text-xs min-h-4" role="alert">
				{#if error}
					{error}
				{/if}
			</p>

			<footer class="w-full">
				<button type="submit" class="btn btn-sm btn-primary text-xs w-full" disabled={saving}>
					{editingId ? 'Save changes' : 'Create profile'}
				</button>
			</footer>
		</form>
	{/if}
{/snippet}

{#snippet list()}
	{#if profiles.length === 0}
		<div class="flex flex-col items-center justify-center text-center">
			<div
				class="bg-primary/10 text-primary mb-4 flex size-9 items-center justify-center rounded-md"
			>
				<UsersRound class="size-4" />
			</div>
			<h2 class="font-semibold">No profiles yet</h2>
			<p class="text-muted-content mt-1 max-w-sm text-xs">
				Create a profile to group identities and define the MCP tools available to them.
			</p>
			<button class="btn btn-primary btn-sm mt-5" onclick={createProfile}>
				<Plus class="size-4" />
				Create profile
			</button>
		</div>
	{:else}
		<div class="grid grid-cols-1 @2xl:grid-cols-2 @4xl:grid-cols-3 gap-4">
			{#each profiles as profile (profile.id)}
				{@const resources = profileResourcePreviews(profile)}
				<article
					class="border-base-300 dark:border-base-400 bg-base-100 dark:bg-base-300 dark:hover:border-primary/80 hover:border-primary/80 group relative rounded-xl border p-4 shadow-sm transition"
				>
					<button
						type="button"
						class="absolute inset-0 rounded-xl"
						aria-label={`Edit ${profile.name}`}
						onclick={() => editProfile(profile)}
					></button>
					<div class="pointer-events-none relative">
						<div class="flex items-start justify-between gap-3">
							<div class="min-w-0">
								<h3 class="truncate font-semibold">{profile.name}</h3>
								{#if profile.users.length > 0}
									<p class="text-muted-content mt-1 flex items-center gap-1.5 text-xs">
										<UsersRound class="size-3 shrink-0" />
										<span class="truncate">{profileUsersDisplayText(profile)}</span>
									</p>
								{/if}
								{#if resources.items.length > 0}
									<ul class="mt-4 flex flex-wrap gap-1">
										{#each resources.items as resource (resource.id)}
											<li
												title={resource.name}
												class="bg-base-100 dark:bg-base-300 border-base-300 dark:border-base-400 group-hover:border-primary/40 flex shrink-0 items-center gap-2 rounded-md border pr-2 transition-colors"
											>
												<McpServerIcon
													icon={resource.icon}
													width={12}
													height={12}
													class="size-3"
													classes={{ root: 'rounded-r-none' }}
												/>
												<span
													class={twMerge(
														'text-xs whitespace-nowrap text-muted-content',
														resource.changed && 'text-base-content'
													)}
												>
													{resource.changed
														? `${resource.enabled} of ${resource.total}`
														: 'Default'}
												</span>
											</li>
										{/each}
										{#if resources.more > 0}
											<li
												class="text-muted-content self-center text-xs font-light badge bg-transparent border-base-300 dark:border-base-400"
											>
												+{resources.more} more
											</li>
										{/if}
									</ul>
								{/if}
							</div>
							<IconButton
								variant="danger"
								class="pointer-events-auto"
								disabled={saving}
								tooltip={{ text: `Delete ${profile.name}`, placement: 'bottom' }}
								onclick={() => promptDelete(profile.id, profile.name)}
							>
								<Trash2 class="size-4" />
							</IconButton>
						</div>
					</div>
				</article>
			{/each}
		</div>
	{/if}
{/snippet}
