<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import { m } from '$lib/i18n';
	import type { GroupAssignment } from './types';

	interface Props {
		groupAssignment?: GroupAssignment;
		loading?: boolean;
		onsuccess: (groupAssignment: GroupAssignment) => void;
		oncancel: () => void;
	}

	let { groupAssignment = $bindable(), loading = false, onsuccess, oncancel }: Props = $props();
</script>

<Confirm
	{loading}
	show={Boolean(groupAssignment)}
	onsuccess={async () => {
		if (!groupAssignment) return;
		onsuccess(groupAssignment);
	}}
	{oncancel}
	type="info"
	title={m.admin_routes_groups_confirm_owner_title()}
	msg={m.admin_routes_groups_assign_owner_msg({ name: `${groupAssignment?.group.name}` })}
>
	{#snippet note()}
		<div class="my-4 flex flex-col gap-4">
			<p class="text-left text-warning">
				{m.admin_routes_groups_owner_warning()}
			</p>
			<div class="text-left text-sm">
				<p class="mb-2">
					{m.admin_routes_groups_owner_members_prefix()}
					<b>{groupAssignment?.group.name}</b>
					{m.admin_routes_groups_owner_members_suffix()}
				</p>
				<ul class="ml-6 list-disc space-y-1">
					<li>{m.admin_routes_groups_owner_ability_manage()}</li>
					<li>{m.admin_routes_groups_owner_ability_assign_roles()}</li>
					<li>{m.admin_routes_groups_owner_ability_assign_auditor()}</li>
					<li>{m.admin_routes_groups_owner_ability_modify_config()}</li>
					<li>{m.admin_routes_groups_owner_ability_delete_users()}</li>
				</ul>
			</div>
			<p class="text-left text-sm">
				{m.admin_routes_groups_owner_implications()}
			</p>
			<p class="text-left font-semibold">
				{m.admin_routes_groups_owner_confirm()}
			</p>
		</div>
	{/snippet}
</Confirm>
