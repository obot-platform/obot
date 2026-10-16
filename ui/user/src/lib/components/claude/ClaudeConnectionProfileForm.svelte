<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import Select from '$lib/components/Select.svelte';
	import SubjectName from '$lib/components/admin/SubjectName.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import VMcpIcon from '$lib/components/vmcps/VMcpIcon.svelte';
	import { m } from '$lib/i18n';
	import {
		UserService,
		type AccessControlRuleSubject,
		type OrgGroup,
		type OrgUser,
		type VMCP
	} from '$lib/services';
	import { doGet, doPut } from '$lib/services/http';
	import { resolveVMcpComponents } from '$lib/services/vmcps/utils';
	import { success } from '$lib/stores/success';
	import {
		OBOT_ADMIN_PICKER_ID,
		resolveSubjectFromGroup,
		resolveSubjectPickerById,
		resolveSubjects
	} from '$lib/subjectResolver';
	import { getUserRoleLabel, isDisabledUser } from '$lib/utils';
	import ActorResources from './ActorResources.svelte';
	import EgressHosts from './EgressHosts.svelte';
	import EgressSecrets from './EgressSecrets.svelte';
	import type { ClaudeConfig, ClaudeEnvironment, EnvironmentProfile } from './types';
	import { ArrowLeft, Trash2, UsersRound, X } from '@lucide/svelte';
	import { onMount, untrack } from 'svelte';
	import { fly } from 'svelte/transition';

	let {
		environment,
		profiles,
		profile,
		editingName,
		servers = [],
		onClose,
		onSaved
	}: {
		environment: ClaudeEnvironment;
		profiles: EnvironmentProfile[];
		profile: EnvironmentProfile;
		editingName?: string;
		servers?: VMCP[];
		onClose: () => void;
		onSaved: (profiles: EnvironmentProfile[]) => void;
	} = $props();

	let draft = $state<EnvironmentProfile>(untrack(() => structuredClone($state.snapshot(profile))));
	let saving = $state(false);
	let error = $state('');
	let directoryUsers = $state<OrgUser[]>([]);
	let directoryGroups = $state<OrgGroup[]>([]);
	let resolvedGroups = $state<OrgGroup[]>([]);
	let subjectQuery = $state('');
	let subjectSelection = $state<string | number>();
	let vmcpQuery = $state('');
	let vmcpSelection = $state<string | number>();
	let confirmDelete = $state<string>();
	let formElement = $state<HTMLFormElement>();
	const initialProfile = untrack(() => canonical(profile));
	const profileDirty = $derived(canonical(draft) !== initialProfile);

	const EVERYONE_GROUP: OrgGroup = { id: '*', name: m.core_all_obot_users() };
	const ADMIN_GROUP: OrgGroup = {
		id: OBOT_ADMIN_PICKER_ID,
		name: m.identity_access_users_obot_admin()
	};
	const GROUP_PAGE_SIZE = 50;
	const assignedSubjectIds = $derived(new Set(draft.subjects.map(resolveSubjectPickerById)));
	const groupsById = $derived(
		new Map([...resolvedGroups, ...directoryGroups].map((group) => [group.id, group]))
	);
	const subjectOptions = $derived.by(() => {
		const search = subjectQuery.trim().toLowerCase();
		const everyoneMatches = !search || EVERYONE_GROUP.name.toLowerCase().includes(search);
		const adminMatches = !search || ADMIN_GROUP.name.toLowerCase().includes(search);
		const selectable = directoryUsers.filter((user) => !isDisabledUser(user));
		const matchingUsers = search
			? selectable.filter(
					(user) =>
						(user.displayName ?? '').toLowerCase().includes(search) ||
						(user.email ?? '').toLowerCase().includes(search) ||
						(user.username ?? '').toLowerCase().includes(search)
				)
			: selectable;
		const matchingGroups = [
			...(everyoneMatches ? [EVERYONE_GROUP] : []),
			...(adminMatches ? [ADMIN_GROUP] : []),
			...directoryGroups
		];
		return [...matchingGroups, ...matchingUsers]
			.filter((entry) => !assignedSubjectIds.has(entry.id))
			.map((entry) => ({
				id: entry.id,
				label: isDirectoryGroup(entry)
					? entry.name
					: [directoryName(entry), entry.email, entry.username].filter(Boolean).join(' · '),
				subject: toSubject(entry)
			}));
	});
	const selectedVmcpIds = $derived(new Set(draft.mcpServerIDs ?? []));
	const vmcpOptions = $derived.by(() => {
		const search = vmcpQuery.trim().toLowerCase();
		return servers
			.filter((server) => !selectedVmcpIds.has(server.id))
			.filter((server) => {
				if (!search) return true;
				return (server.displayName || server.id).toLowerCase().includes(search);
			})
			.map((server) => ({
				id: server.id,
				label: server.displayName || server.id
			}));
	});

	let groupsRequest: AbortController | undefined;

	onMount(() => {
		void loadUsers();
	});

	$effect(() => {
		const search = subjectQuery;
		const timer = setTimeout(() => loadGroups(search), search ? 500 : 0);
		return () => {
			clearTimeout(timer);
			groupsRequest?.abort();
		};
	});

	$effect(() => {
		const subjects = draft.subjects;
		if (!subjects.some((subject) => subject.type !== 'selector')) return;

		const controller = new AbortController();
		resolveSubjects(
			subjects,
			untrack(() => ({
				users: directoryUsers.length > 0 ? directoryUsers : undefined,
				groups: resolvedGroups
			})),
			{ signal: controller.signal }
		)
			.then((resolved) => {
				if (controller.signal.aborted) return;
				directoryUsers = resolved.users;
				rememberResolvedGroups(resolved.groups);
			})
			.catch((err) => {
				if (controller.signal.aborted) return;
				console.error('Failed to resolve identities:', err);
			});
		return () => controller.abort();
	});

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
		return resolveSubjectFromGroup(entry);
	}

	function rememberResolvedGroups(groups: OrgGroup[]) {
		if (groups.length === 0) return;
		// eslint-disable-next-line svelte/prefer-svelte-reactivity
		const next = new Map(resolvedGroups.map((group) => [group.id, group]));
		let changed = false;
		for (const group of groups) {
			if (next.get(group.id)?.name === group.name) continue;
			next.set(group.id, group);
			changed = true;
		}
		if (changed) resolvedGroups = [...next.values()];
	}

	function subjectDisplay(subject: AccessControlRuleSubject) {
		if (subject.type === 'obotGroup') {
			return {
				name: `Obot ${subject.id.charAt(0).toUpperCase()}${subject.id.slice(1)}`,
				group: true,
				iconURL: undefined,
				role: undefined,
				disabled: false
			};
		}
		if (subject.type === 'selector') {
			const name = subject.id === EVERYONE_GROUP.id ? EVERYONE_GROUP.name : subject.id;
			return { name, group: true, iconURL: undefined, role: undefined, disabled: false };
		}
		if (subject.type === 'group') {
			const group = groupsById.get(subject.id);
			return {
				name: group?.name || subject.id,
				group: true,
				iconURL: undefined,
				role: undefined,
				disabled: false
			};
		}
		const user = directoryUsers.find((candidate) => candidate.id === subject.id);
		return {
			name: user ? directoryName(user) : subject.id,
			group: false,
			iconURL: user?.iconURL,
			role: user?.effectiveRole,
			disabled: isDisabledUser(user)
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
		if (draft.subjects.some((item) => item.type === subject.type && item.id === subject.id)) return;
		draft.subjects = [...draft.subjects, subject];
	}

	function removeSubject(subject: AccessControlRuleSubject) {
		draft.subjects = draft.subjects.filter(
			(candidate) => !(candidate.type === subject.type && candidate.id === subject.id)
		);
	}

	function addVmcp(id: string) {
		vmcpSelection = undefined;
		if (!id) return;
		const current = draft.mcpServerIDs ?? [];
		if (current.includes(id)) return;
		draft.mcpServerIDs = [...current, id];
	}

	function removeVmcp(id: string) {
		if (!draft.mcpServerIDs) return;
		draft.mcpServerIDs = draft.mcpServerIDs.filter((candidate) => candidate !== id);
	}

	async function loadUsers() {
		if (directoryUsers.length > 0) return;
		try {
			directoryUsers = await UserService.listUsers();
		} catch (err) {
			console.error('Failed to load users:', err);
		}
	}

	async function loadGroups(search: string) {
		groupsRequest?.abort();
		const controller = new AbortController();
		groupsRequest = controller;
		try {
			const page = await UserService.listGroups({
				query: search.trim() || undefined,
				limit: GROUP_PAGE_SIZE,
				signal: controller.signal
			});
			if (controller.signal.aborted) return;
			directoryGroups = [...page.items].sort((a, b) => a.name.localeCompare(b.name));
			rememberResolvedGroups(directoryGroups);
		} catch (err) {
			if (controller.signal.aborted) return;
			console.error('Failed to load groups:', err);
			directoryGroups = [];
		}
	}

	function optionalNumber(value: number | undefined | null) {
		if (value === undefined || value === null || Number.isNaN(value)) return null;
		return value;
	}

	function canonical(value: EnvironmentProfile) {
		const copy = structuredClone($state.snapshot(value));
		return JSON.stringify({
			name: (copy.name ?? '').trim(),
			subjects: copy.subjects ?? [],
			mcpServerIDs: copy.mcpServerIDs,
			allowAllEgress: Boolean(copy.allowAllEgress),
			allowClaudeMCP: Boolean(copy.allowClaudeMCP),
			egressHosts: (copy.egressHosts ?? []).map((host) => host.trim()).filter(Boolean),
			egressSecrets: (copy.egressSecrets ?? []).map((secret) => ({
				id: secret.id ?? '',
				hostname: (secret.hostname ?? '').trim(),
				header: (secret.header ?? '').trim(),
				value: secret.value ?? ''
			})),
			resources: {
				actorCPU: (copy.resources?.actorCPU ?? '').trim(),
				actorMemory: (copy.resources?.actorMemory ?? '').trim(),
				idleTimeoutMinutes: optionalNumber(copy.resources?.idleTimeoutMinutes),
				sessionRetentionDays: optionalNumber(copy.resources?.sessionRetentionDays)
			}
		});
	}

	function normalize(value: EnvironmentProfile): EnvironmentProfile {
		const copy = structuredClone($state.snapshot(value));
		copy.name = copy.name.trim();
		copy.egressHosts = copy.egressHosts?.map((host) => host.trim()).filter(Boolean);
		return copy;
	}

	async function persist(next: EnvironmentProfile[]) {
		if (!environment.id) return next;
		saving = true;
		error = '';
		try {
			const latest = (await doGet('/claude-environments')) as ClaudeConfig;
			const current = latest.environments?.find((item) => item.id === environment.id);
			if (!current) throw new Error(m.hosted_agents_env_not_found());
			const updated = (await doPut(`/claude-environments/${current.id}`, {
				...structuredClone(current),
				organizationID: undefined,
				environmentID: undefined,
				profiles: next
			})) as ClaudeEnvironment;
			return updated.profiles;
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
			return false;
		} finally {
			saving = false;
		}
	}

	async function commitProfile() {
		if (saving) return false;
		const name = draft.name.trim();
		if (!name) {
			error = m.hosted_agents_profile_name_required();
			return false;
		}
		if (profiles.some((item) => item.name !== editingName && item.name.trim() === name)) {
			error = m.hosted_agents_profile_name_exists();
			return false;
		}
		const saved = normalize({ ...draft, name });
		const next = editingName
			? profiles.map((item) => (item.name === editingName ? saved : item))
			: [...profiles, saved];
		const updated = await persist(next);
		if (!updated) return false;
		if (environment.id) success.add(m.hosted_agents_profile_saved({ name }));
		onSaved(updated);
		return true;
	}

	async function saveProfile(event: SubmitEvent) {
		event.preventDefault();
		await commitProfile();
	}

	export function hasUnsavedChanges() {
		return profileDirty;
	}

	export function checkValidity() {
		return formElement?.checkValidity() ?? false;
	}

	export function reportValidity() {
		formElement?.reportValidity();
	}

	export async function applyUnsaved() {
		if (!checkValidity()) return false;
		return commitProfile();
	}

	async function deleteProfile(name: string) {
		if (saving) return;
		const next = profiles.filter((item) => item.name !== name);
		const updated = await persist(next);
		if (!updated) return;
		success.add(m.hosted_agents_profile_deleted({ name }));
		confirmDelete = undefined;
		onSaved(updated);
	}
</script>

<form
	bind:this={formElement}
	class="mx-auto flex w-full max-w-4xl flex-col gap-3 rounded-xl border border-base-300 bg-base-100 p-5 shadow-sm dark:bg-base-300"
	onsubmit={saveProfile}
>
	<header class="flex items-start gap-3">
		<IconButton
			disabled={saving}
			tooltip={{ text: m.hosted_agents_profile_back() }}
			onclick={onClose}
		>
			<ArrowLeft class="size-4" />
		</IconButton>
		<div class="min-w-0 grow">
			<h2 class="text-md font-semibold">
				{editingName ? m.hosted_agents_profile_edit() : m.hosted_agents_profile_create()}
			</h2>
			<p class="text-muted-content text-sm">
				{m.hosted_agents_profile_description()}
			</p>
		</div>
		{#if editingName}
			<IconButton
				variant="danger"
				disabled={saving}
				tooltip={{ text: m.hosted_agents_profile_remove() }}
				onclick={() => (confirmDelete = editingName)}
			>
				<Trash2 class="size-4" />
			</IconButton>
		{/if}
	</header>
	<div class="divider my-0"></div>
	{#if error}<p role="alert" class="text-error text-sm">{error}</p>{/if}
	<fieldset disabled={saving} class="space-y-3">
		<label class="flex min-w-0 flex-col gap-1 text-sm font-light">
			{m.hosted_agents_profile_name()}
			<input class="text-input-filled w-full" required bind:value={draft.name} />
		</label>

		<section>
			<div class="divider text-sm font-semibold mt-5 mb-3">{m.vmcps()}</div>
			<Select
				id="profile-vmcps"
				options={vmcpOptions}
				bind:query={vmcpQuery}
				bind:selected={vmcpSelection}
				searchInDropdown
				placeholder={m.hosted_agents_select_vmcps()}
				searchPlaceholder={m.hosted_agents_search_vmcps()}
				class="bg-base-200 shadow-inner!"
				classes={{ root: 'w-full' }}
				onSelect={(option) => addVmcp(String(option.id))}
				disabled={saving}
			/>
			{#if draft.mcpServerIDs === null || draft.mcpServerIDs.length === 0}
				<div class="text-muted-content pb-4 pt-3 text-center text-xs font-light italic">
					{m.hosted_agents_profile_no_vmcps()}
				</div>
			{:else if draft.mcpServerIDs}
				<div class="mt-2 flex flex-col">
					{#each draft.mcpServerIDs as id, index (id)}
						{@const server = servers.find((item) => item.id === id)}
						<div class="flex items-center justify-between p-2" out:fly={{ x: 100, duration: 100 }}>
							<div class="flex min-w-0 items-center gap-2">
								<VMcpIcon class="size-8" components={server ? resolveVMcpComponents(server) : []} />
								<span class="truncate text-sm font-light">{server?.displayName || id}</span>
							</div>
							<IconButton
								class="size-8"
								tooltip={{
									text: m.hosted_agents_profile_remove_subject({
										name: server?.displayName || id
									})
								}}
								disabled={saving}
								onclick={() => removeVmcp(id)}
								variant="danger"
							>
								<X class="size-4" />
							</IconButton>
						</div>
						{#if index < draft.mcpServerIDs.length - 1}
							<div class="divider my-0 px-2 before:h-px after:h-px"></div>
						{/if}
					{/each}
				</div>
			{/if}
		</section>

		<fieldset class="space-y-4">
			<div class="divider text-sm font-semibold mb-3">{m.hosted_agents_profile_egress()}</div>
			<div class="notification-info">
				<p class="text-muted-content text-xs font-light">
					{m.hosted_agents_required_egress_help()}
				</p>
				<p class="text-muted-content mt-2 text-xs font-light">
					{m.hosted_agents_profile_egress_help()}
				</p>
			</div>
			<label
				for="allow-claude-mcp"
				class="flex items-center justify-between gap-4 rounded-lg border border-base-300 p-4 dark:border-base-400"
			>
				<div>
					<p class="text-sm font-semibold">{m.hosted_agents_allow_claude_mcp()}</p>
					<p class="text-muted-content text-xs leading-tight font-light">
						{m.hosted_agents_allow_claude_mcp_help()}
					</p>
				</div>
				<input
					id="allow-claude-mcp"
					type="checkbox"
					class="toggle toggle-sm shrink-0"
					bind:checked={draft.allowClaudeMCP}
					disabled={saving}
					aria-label={m.hosted_agents_allow_claude_mcp()}
				/>
			</label>
			<label
				for="allow-all-egress"
				class="flex items-center justify-between gap-4 rounded-lg border border-base-300 p-4 dark:border-base-400"
			>
				<div>
					<p class="text-sm font-semibold">{m.hosted_agents_allow_all_egress()}</p>
					<p class="text-muted-content text-xs leading-tight font-light">
						{m.hosted_agents_allow_all_egress_help()}
					</p>
				</div>
				<input
					id="allow-all-egress"
					type="checkbox"
					class="toggle toggle-sm shrink-0"
					bind:checked={draft.allowAllEgress}
					disabled={saving}
					aria-label={m.hosted_agents_allow_all_egress()}
				/>
			</label>
			{#if !draft.allowAllEgress}
				<div class="rounded-lg border border-base-300 p-4 dark:border-base-400">
					<EgressHosts
						classes={{ header: 'text-sm font-semibold' }}
						label={m.hosted_agents_profile_egress_hosts()}
						bind:hosts={
							() => draft.egressHosts ?? [],
							(hosts) => {
								draft.egressHosts = hosts;
							}
						}
					/>
				</div>
			{/if}
		</fieldset>
		<EgressSecrets
			classes={{ header: 'text-sm font-semibold', root: 'bg-transparent p-4' }}
			bind:secrets={
				() => draft.egressSecrets ?? [],
				(secrets) => {
					draft.egressSecrets = secrets;
				}
			}
		/>
	</fieldset>

	<section>
		<div class="divider my-3 text-sm font-semibold">
			{m.hosted_agents_profile_identities()}
		</div>
		<div class="mb-4">
			<p class="text-muted-content text-sm font-light">
				{m.hosted_agents_profile_identities_description()}
			</p>
		</div>
		<Select
			id="environment-profile-subjects"
			options={subjectOptions}
			bind:query={subjectQuery}
			bind:selected={subjectSelection}
			searchInDropdown
			placeholder={m.hosted_agents_profile_add_subject()}
			searchPlaceholder={m.hosted_agents_profile_search_subject()}
			class="bg-base-200 shadow-inner!"
			classes={{ root: 'w-full' }}
			onSelect={(option) => addSubject(option.subject)}
			disabled={saving}
		/>
		{#if draft.subjects.length === 0}
			<div class="text-muted-content pb-4 pt-3 text-center text-xs font-light italic">
				{m.hosted_agents_profile_no_identities()}
			</div>
		{:else}
			<div class="mt-2 flex flex-col">
				{#each draft.subjects as subject, index (`${subject.type}:${subject.id}`)}
					{@const display = subjectDisplay(subject)}
					<div class="flex items-center justify-between p-2" out:fly={{ x: 100, duration: 100 }}>
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
								<SubjectName
									class="text-sm font-light"
									name={display.name}
									disabled={display.disabled}
								/>
								<span class="text-muted-content text-xs">
									{display.group
										? m.core_col_group()
										: display.role
											? getUserRoleLabel(display.role)
											: m.core_col_user()}
								</span>
							</div>
						</div>
						<IconButton
							class="size-8"
							tooltip={{
								text: m.hosted_agents_profile_remove_subject({ name: display.name })
							}}
							disabled={saving}
							onclick={() => removeSubject(subject)}
							variant="danger"
						>
							<X class="size-4" />
						</IconButton>
					</div>
					{#if index < draft.subjects.length - 1}
						<div class="divider my-0 px-2 before:h-px after:h-px"></div>
					{/if}
				{/each}
			</div>
		{/if}
	</section>

	<div class="divider my-0"></div>
	<details>
		<summary class="cursor-pointer text-sm font-semibold">
			{m.hosted_agents_profile_actor_overrides()}
		</summary>
		<div class="mt-3 space-y-3">
			<p class="text-muted-content text-xs">{m.hosted_agents_profile_actor_help()}</p>
			<ActorResources bind:resources={draft.resources} inherit />
		</div>
	</details>

	<div class="flex justify-end gap-2">
		<button class="btn btn-ghost" type="button" disabled={saving} onclick={onClose}>
			{m.common_cancel()}
		</button>
		<button class="btn btn-primary" type="submit" disabled={saving}>
			{saving ? m.hosted_agents_map_saving() : environment.id ? m.core_save() : m.common_apply()}
		</button>
	</div>
</form>

<Confirm
	show={confirmDelete !== undefined}
	onsuccess={() => {
		if (confirmDelete) void deleteProfile(confirmDelete);
	}}
	oncancel={() => (confirmDelete = undefined)}
	msg={m.hosted_agents_profile_delete_question({ name: confirmDelete ?? '' })}
	loading={saving}
	title={m.hosted_agents_profile_remove()}
/>
