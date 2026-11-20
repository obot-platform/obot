<script lang="ts">
	import { m } from '$lib/i18n';
	import { groupRoleOptions } from '$lib/services/admin/constants.js';
	import { Group, Role } from '$lib/services/admin/types';
	import { profile } from '$lib/stores/index.js';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		roleId: number;
		hasAuditorPrivilege?: boolean;
		hasReadOnlyAdmin?: boolean;
		onRoleChange?: (roleId: number) => void;
		onAuditorChange?: (hasAuditor: boolean) => void;
	}

	interface RoleOption {
		id: number;
		label: string;
	}

	let {
		roleId = $bindable(),
		hasAuditorPrivilege = $bindable(false),
		hasReadOnlyAdmin = $bindable(false),
		onRoleChange,
		onAuditorChange
	}: Props = $props();

	const canAssignOwner = $derived(profile.current.groups.includes(Group.OWNER));
	const canAssignAdmin = $derived(canAssignOwner || profile.current.groups.includes(Group.ADMIN));

	let roleOptions: RoleOption[] = $derived([
		...(hasReadOnlyAdmin || roleId === 0 ? [{ label: m.core_col_none(), id: 0 }] : []),
		...(canAssignOwner ? [{ label: m.core_role_owner(), id: Role.OWNER }] : []),
		...groupRoleOptions
			.filter((role) => (role.id === Role.ADMIN ? canAssignAdmin : true))
			.map((d) => ({ id: d.id, label: d.label }))
	]);

	const roleDescriptionMap = $derived(
		groupRoleOptions.reduce(
			(acc, role) => {
				acc[role.id] = role.description;
				return acc;
			},
			{} as Record<number, string>
		)
	);

	const auditorReadonlyAdminRoles = [Role.BASIC, Role.POWERUSER, Role.POWERUSER_PLUS];

	function handleRoleChange() {
		onRoleChange?.(roleId);
	}

	function handleAuditorChange() {
		onAuditorChange?.(hasAuditorPrivilege);
	}
</script>

{#snippet roleUi(role: RoleOption)}
	<label
		class="border-base-400 hover:bg-base-100/2 active:bg-base-100/5 flex cursor-pointer gap-4 rounded-lg border p-3"
	>
		<input
			type="radio"
			value={role.id}
			bind:group={roleId}
			onchange={handleRoleChange}
			disabled={!profile.current.groups.includes(Group.OWNER) && role.id === Role.OWNER}
		/>
		<div
			class="flex flex-col"
			class:opacity-50={!profile.current.groups.includes(Group.OWNER) && role.id === Role.OWNER}
		>
			<div class="w-28 shrink-0 font-semibold whitespace-nowrap">{role.label}</div>
			<p class="text-muted-content text-xs">
				{#if role.id === Role.OWNER}
					{m.identity_access_groups_owner_description()}
				{:else if role.id === Role.ADMIN}
					{m.identity_access_groups_admin_description()}
				{:else if role.id !== 0}
					{roleDescriptionMap[role.id] ||
						m.identity_access_groups_role_privileges({ role: role.label })}
				{/if}
			</p>
		</div>
	</label>
{/snippet}

<div class="flex flex-col gap-2 text-sm font-light">
	{#each roleOptions as role (role.id)}
		{@render roleUi(role)}
	{/each}

	{#if canAssignAdmin}
		<label class="border-base-400 flex cursor-pointer gap-4 rounded-lg border p-3">
			<input type="checkbox" bind:checked={hasReadOnlyAdmin} />
			<span class="flex flex-col">
				<span class="font-semibold">{m.core_role_admin_readonly()}</span>
				<span class="text-muted-content text-xs">
					{roleId & (Role.ADMIN | Role.OWNER)
						? m.core_role_admin_readonly_redundant()
						: m.core_role_admin_readonly_description()}
				</span>
			</span>
		</label>
	{/if}

	{#if profile.current.groups.includes(Group.OWNER)}
		{@const isDisabled = roleId === 0 && !hasReadOnlyAdmin && !hasAuditorPrivilege}
		<label
			class={twMerge(
				'border-base-400 hover:bg-base-100/2 active:bg-base-100/5 my-4 flex cursor-pointer gap-4 rounded-lg border p-3',
				isDisabled ? 'pointer-events-none opacity-50' : ''
			)}
			aria-disabled={isDisabled}
		>
			<input
				type="checkbox"
				bind:checked={hasAuditorPrivilege}
				onchange={handleAuditorChange}
				disabled={isDisabled}
			/>
			<div class="flex flex-col">
				<div class="w-28 shrink-0 font-semibold">{m.core_role_auditor()}</div>
				<p class="text-muted-content text-xs">
					{#if !hasReadOnlyAdmin && auditorReadonlyAdminRoles.includes(roleId)}
						{m.identity_access_groups_auditor_readonly_description()}
					{:else}
						{m.identity_access_groups_auditor_description()}
					{/if}
				</p>
			</div>
		</label>
	{/if}
</div>
