<script lang="ts">
	import Select from '$lib/components/Select.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import { m } from '$lib/i18n';
	import {
		UserService,
		Group,
		type OrgUser,
		type OrgGroup,
		type AccessControlRuleSubject
	} from '$lib/services';
	import { convertSubjectsToTableData, resolveSubjects } from '$lib/subjectResolver';
	import ActorResources from './ActorResources.svelte';
	import EgressHosts from './EgressHosts.svelte';
	import EgressSecrets from './EgressSecrets.svelte';
	import type { EnvironmentProfile } from './types';
	import { Plus, Trash2, X, Users } from '@lucide/svelte';
	import { onMount } from 'svelte';

	let {
		profiles = $bindable([]),
		disabled = false,
		servers = []
	}: {
		profiles: EnvironmentProfile[];
		disabled?: boolean;
		servers?: { id: string; displayName?: string }[];
	} = $props();
	let users = $state<OrgUser[]>([]);
	let groups = $state<OrgGroup[]>([]);
	let resolvedGroups = $state<OrgGroup[]>([]);
	let query = $state('');
	let error = $state('');
	const admin: AccessControlRuleSubject = { type: 'obotGroup', id: Group.ADMIN };
	const everyone: AccessControlRuleSubject = { type: 'selector', id: '*' };
	const options = $derived([
		{ id: 'admin', label: m.hosted_agents_profile_admin(), subject: admin },
		{ id: 'everyone', label: m.hosted_agents_profile_everyone(), subject: everyone },
		...users.map((user) => ({
			id: `user:${user.id}`,
			label: `${user.displayName || user.username} (${user.email})`,
			subject: { type: 'user', id: user.id } as AccessControlRuleSubject
		})),
		...groups.map((group) => ({
			id: `group:${group.id}`,
			label: group.name,
			subject: { type: 'group', id: group.id } as AccessControlRuleSubject
		}))
	]);
	onMount(() => {
		void UserService.listUsers()
			.then((directoryUsers) => {
				users = directoryUsers;
				return resolveSubjects(
					profiles.flatMap((profile) => profile.subjects),
					{ users: directoryUsers }
				);
			})
			.then((result) => {
				users = result.users;
				resolvedGroups = result.groups;
			})
			.catch((e) => (error = String(e)));
	});
	$effect(() => {
		const search = query;
		const controller = new AbortController();
		const timer = setTimeout(
			() => {
				void UserService.listGroups({
					query: search || undefined,
					limit: 50,
					signal: controller.signal
				})
					.then((result) => {
						if (controller.signal.aborted) return;
						groups = result.items;
						resolvedGroups = [
							...new Map([...resolvedGroups, ...groups].map((group) => [group.id, group])).values()
						];
					})
					.catch((e) => {
						if (!controller.signal.aborted) error = String(e);
					});
			},
			search ? 300 : 0
		);
		return () => {
			clearTimeout(timer);
			controller.abort();
		};
	});
	function add(index: number, subject: AccessControlRuleSubject) {
		if (!profiles[index].subjects.some((s) => s.type === subject.type && s.id === subject.id)) {
			profiles[index].subjects = [...profiles[index].subjects, subject];
		}
	}
</script>

<fieldset {disabled} class="space-y-3">
	<legend class="mb-1 text-sm font-semibold">{m.hosted_agents_profiles_title()}</legend>
	<p class="text-sm font-light text-muted-content">
		{m.hosted_agents_profiles_help()}
	</p>
	{#if error}<p role="alert" class="text-error text-sm">{error}</p>{/if}
	{#each profiles as profile, index (index)}
		<div class="space-y-3 rounded-lg border border-base-300 bg-base-200/50 p-3">
			<div class="flex items-end gap-2">
				<label class="flex min-w-0 flex-1 flex-col gap-1 text-sm font-light"
					>{m.hosted_agents_profile_name()}<input
						class="text-input-filled w-full"
						required
						bind:value={profile.name}
					/></label
				>
				<IconButton
					variant="danger"
					tooltip={{ text: m.hosted_agents_profile_remove() }}
					{disabled}
					onclick={() => (profiles = profiles.filter((_, i) => i !== index))}
					><Trash2 class="size-4" /></IconButton
				>
			</div>
			<Select
				id={`environment-profile-${index}`}
				{options}
				bind:query
				searchInDropdown
				placeholder={m.hosted_agents_profile_add_subject()}
				searchPlaceholder={m.hosted_agents_profile_search_subject()}
				class="bg-base-200 shadow-inner!"
				classes={{ root: 'w-full' }}
				onSelect={(option) => add(index, option.subject)}
				{disabled}
			/>
			<ul class="flex flex-wrap gap-2">
				{#each convertSubjectsToTableData(profile.subjects, users, resolvedGroups) as subject, subjectIndex (subjectIndex)}
					<li
						class="flex max-w-full items-center gap-1 rounded-lg border border-base-300 bg-base-100 py-1 pl-2 pr-1 text-xs"
					>
						<Users class="mr-1 size-3.5 shrink-0 text-muted-content" /><span class="break-words"
							>{subject.displayName}</span
						>
						<IconButton
							class="btn-xs"
							tooltip={{
								text: m.hosted_agents_profile_remove_subject({ name: subject.displayName })
							}}
							{disabled}
							onclick={() =>
								(profile.subjects = profile.subjects.filter((_, i) => i !== subjectIndex))}
							><X class="size-3" /></IconButton
						>
					</li>
				{:else}<li class="text-xs font-light text-muted-content">
						{m.hosted_agents_profile_no_subjects()}
					</li>{/each}
			</ul>
			<Select
				id={`profile-vmcps-${index}`}
				options={servers
					.toSorted(
						(a, b) =>
							Number(profile.mcpServerIDs?.includes(b.id)) -
							Number(profile.mcpServerIDs?.includes(a.id))
					)
					.map((server) => ({
						id: server.id,
						label: server.displayName || server.id
					}))}
				bind:selected={
					() => profile.mcpServerIDs?.join(',') ?? '',
					(value) =>
						(profile.mcpServerIDs = String(value ?? '')
							.split(',')
							.filter(Boolean))
				}
				multiple
				searchInDropdown
				buttonReadOnly
				placeholder={m.hosted_agents_select_vmcps()}
				searchPlaceholder={m.hosted_agents_search_vmcps()}
				classes={{ root: 'w-full' }}
				{disabled}
			/>
			<fieldset class="space-y-2">
				<legend class="text-sm font-semibold">{m.hosted_agents_profile_egress()}</legend>
				<p class="text-xs text-muted-content">{m.hosted_agents_profile_egress_help()}</p>
				<label class="flex items-center gap-2 text-sm">
					<input
						class="checkbox checkbox-sm"
						type="checkbox"
						bind:checked={profile.allowAllEgress}
					/>
					{m.hosted_agents_allow_all_egress()}
				</label>
				<EgressHosts
					label={m.hosted_agents_profile_egress_hosts()}
					bind:hosts={() => profile.egressHosts ?? [], (hosts) => (profile.egressHosts = hosts)}
					disabled={profile.allowAllEgress}
				/>
				<p class="text-xs text-muted-content">{m.hosted_agents_required_egress_help()}</p>
			</fieldset>
			<EgressSecrets
				bind:secrets={
					() => profile.egressSecrets ?? [], (secrets) => (profile.egressSecrets = secrets)
				}
			/>
			<details>
				<summary class="cursor-pointer text-sm font-semibold"
					>{m.hosted_agents_profile_actor_overrides()}</summary
				>
				<div class="mt-3 space-y-3">
					<p class="text-xs text-muted-content">
						{m.hosted_agents_profile_actor_help()}
					</p>
					<ActorResources bind:resources={profile.resources} inherit />
				</div>
			</details>
		</div>
	{/each}
	<button
		type="button"
		class="btn btn-ghost btn-sm"
		onclick={() =>
			(profiles = [...profiles, { name: '', subjects: [], mcpServerIDs: [], resources: {} }])}
		><Plus class="size-4" />{m.hosted_agents_profile_add()}</button
	>
</fieldset>
