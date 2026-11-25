<script module lang="ts">
	import type { AdminAuditEventExportFilters } from '$lib/services';

	// The admin export filters as the form edits them: each a comma-separated list.
	export type AdminExportFilterValues = {
		resourceKinds: string;
		resourceIDs: string;
		actors: string;
		actions: string;
	};

	export function toAdminExportFilterValues(
		filters?: AdminAuditEventExportFilters
	): AdminExportFilterValues {
		return {
			resourceKinds: filters?.resourceKinds?.join(',') ?? '',
			resourceIDs: filters?.resourceIDs?.join(',') ?? '',
			actors: filters?.actors?.join(',') ?? '',
			actions: filters?.actions?.join(',') ?? ''
		};
	}

	export function toAdminAuditEventExportFilters(
		values: AdminExportFilterValues
	): AdminAuditEventExportFilters {
		const split = (value: string) =>
			value
				.split(',')
				.map((s) => s.trim())
				.filter((s) => s.length > 0);
		return {
			resourceKinds: split(values.resourceKinds),
			resourceIDs: split(values.resourceIDs),
			actors: split(values.actors),
			actions: split(values.actions)
		};
	}

	// The actions the admin change audit records, as pkg/gateway/types/adminauditevent.go names them.
	const ADMIN_AUDIT_ACTIONS = [
		'agent.create',
		'agent.update',
		'agent.disable',
		'agent.enable',
		'agent.delete',
		'agent.key.create',
		'agent.key.revoke',
		'model_access_policy.subjects.update',
		'skill_access_rule.subjects.update',
		'vmcp.profile.subjects.update',
		'vmcp.preconfigured_oauth.authorize',
		'vmcp.preconfigured_oauth.revoke',
		'vmcp.preconfigured_oauth.invalidate',
		'vmcp.preconfigured_oauth.delete'
	];

	// The actor of the changes Obot makes itself, such as invalidating a preconfigured OAuth grant.
	const SYSTEM_ACTOR = 'system';
</script>

<script lang="ts">
	import Select from '$lib/components/Select.svelte';
	import { m } from '$lib/i18n';
	import type { OrgUser } from '$lib/services';
	import { getUserDisplayName, withDisabledMarker } from '$lib/utils';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		values: AdminExportFilterValues;
		users: Map<string, OrgUser>;
		disabled?: boolean;
	}

	let { values = $bindable(), users, disabled = false }: Props = $props();

	const resourceKindOptions = [
		{ id: 'agent', label: m.audit_usage_agent() },
		{ id: 'apiKey', label: m.audit_usage_exports_admin_resource_kind_api_key() },
		// A vMCP's preconfigured OAuth is recorded against the vMCP, with the component in the details.
		{ id: 'vmcp', label: m.audit_usage_exports_admin_resource_kind_vmcp() },
		{ id: 'vmcpProfile', label: m.audit_usage_exports_admin_resource_kind_vmcp_profile() },
		{
			id: 'modelAccessPolicy',
			label: m.audit_usage_exports_admin_resource_kind_model_access_policy()
		},
		{ id: 'skillAccessRule', label: m.audit_usage_exports_admin_resource_kind_skill_access_rule() }
	];
	const actionOptions = ADMIN_AUDIT_ACTIONS.map((action) => ({ id: action, label: action }));
	let actorOptions = $derived([
		{ id: SYSTEM_ACTOR, label: m.audit_usage_exports_admin_actor_system() },
		...[...users.values()].map((user) => ({
			id: user.id,
			label: withDisabledMarker(getUserDisplayName(users, user.id), user)
		}))
	]);

	const selectClasses = { root: 'w-full', clear: 'hover:bg-base-400 bg-transparent' };
	let selectClass = $derived(twMerge(disabled && 'text-[currentColor] disabled:opacity-100'));
</script>

<div class="grid grid-cols-1 gap-4 md:grid-cols-2">
	<div class="flex flex-col gap-1">
		<label class="text-sm font-medium" id="admin_resource_kind-label" for="admin_resource_kind"
			>{m.audit_usage_exports_admin_filter_title_resource_kinds()}</label
		>
		<Select
			id="admin_resource_kind"
			ariaLabelledby="admin_resource_kind-label"
			class={selectClass}
			classes={selectClasses}
			options={resourceKindOptions}
			bind:selected={() => values.resourceKinds, (v) => (values.resourceKinds = String(v ?? ''))}
			{disabled}
			readonly={disabled}
			multiple
		/>
		<p class="text-muted-content text-xs">
			{m.audit_usage_exports_admin_filter_desc_resource_kinds()}
		</p>
	</div>
	<div class="flex flex-col gap-1">
		<label class="text-sm font-medium" id="admin_action-label" for="admin_action"
			>{m.audit_usage_exports_admin_filter_title_actions()}</label
		>
		<Select
			id="admin_action"
			ariaLabelledby="admin_action-label"
			class={selectClass}
			classes={selectClasses}
			options={actionOptions}
			bind:selected={() => values.actions, (v) => (values.actions = String(v ?? ''))}
			{disabled}
			readonly={disabled}
			multiple
		/>
		<p class="text-muted-content text-xs">{m.audit_usage_exports_admin_filter_desc_actions()}</p>
	</div>
	<div class="flex flex-col gap-1">
		<label class="text-sm font-medium" id="admin_actor-label" for="admin_actor"
			>{m.audit_usage_exports_admin_filter_title_actors()}</label
		>
		<Select
			id="admin_actor"
			ariaLabelledby="admin_actor-label"
			class={selectClass}
			classes={selectClasses}
			options={actorOptions}
			bind:selected={() => values.actors, (v) => (values.actors = String(v ?? ''))}
			{disabled}
			readonly={disabled}
			multiple
		/>
		<p class="text-muted-content text-xs">{m.audit_usage_exports_admin_filter_desc_actors()}</p>
	</div>
	<div class="flex flex-col gap-1">
		<label class="text-sm font-medium" for="admin_resource_id"
			>{m.audit_usage_exports_admin_filter_title_resource_ids()}</label
		>
		<input
			id="admin_resource_id"
			class={twMerge('text-input-filled', disabled && 'text-[currentColor] disabled:opacity-100')}
			bind:value={values.resourceIDs}
			placeholder="7, 12"
			readonly={disabled}
			{disabled}
		/>
		<p class="text-muted-content text-xs">
			{m.audit_usage_exports_admin_filter_desc_resource_ids()}
		</p>
	</div>
</div>
