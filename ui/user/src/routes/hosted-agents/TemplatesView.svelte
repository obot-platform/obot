<script lang="ts">
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import HostedAgentForm from '$lib/components/admin/HostedAgentForm.svelte';
	import AgentIcon from '$lib/components/hosted-agents/AgentIcon.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { m } from '$lib/i18n';
	import { type Harness, type HostedAgent } from '$lib/services/admin/types';
	import { AdminService } from '$lib/services/index.js';
	import { profile } from '$lib/stores/index.js';
	import { clearUrlParams, goto } from '$lib/url';
	import { openUrl } from '$lib/utils.js';
	import { Bot, Plus, Trash2 } from '@lucide/svelte';
	import { untrack } from 'svelte';
	import { fade, fly } from 'svelte/transition';

	interface Props {
		templates: HostedAgent[];
		harnesses: Harness[];
		creating?: boolean;
	}

	let { templates: initialTemplates, harnesses, creating = false }: Props = $props();
	let hostedAgents = $state(untrack(() => initialTemplates));
	let agentToDelete = $state<HostedAgent>();
	let isReadonly = $derived(profile.current.isAdminReadonly?.());
	const duration = PAGE_TRANSITION_DURATION;

	let harnessesById = $derived(new Map(harnesses.map((h) => [h.id, h])));
	let tableData = $derived(
		hostedAgents.map((agent) => ({
			id: agent.id,
			name: agent.name,
			harness: harnessesById.get(agent.harnessID)?.name ?? agent.harnessID,
			icon: agent.icon || harnessesById.get(agent.harnessID)?.icon || '',
			iconDark: agent.iconDark || harnessesById.get(agent.harnessID)?.iconDark || ''
		}))
	);

	async function navigateToCreated(agent: HostedAgent) {
		clearUrlParams(['new']);
		goto(`/hosted-agents/${agent.id}`, { replaceState: false });
	}
</script>

{#if creating}
	<div class="h-full w-full" in:fly|global={{ x: 100, delay: duration, duration }}>
		<HostedAgentForm onCreate={navigateToCreated} readonly={isReadonly} />
	</div>
{:else}
	<div class="flex flex-col gap-4" in:fade={{ duration }}>
		<p class="text-muted-content text-sm font-light">
			{m.hosted_agents_templates_desc()}
		</p>

		{#if hostedAgents.length === 0}
			<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
				<Bot class="text-muted-content size-24 opacity-25" />
				<h4 class="text-muted-content text-lg font-semibold">
					{m.hosted_agents_templates_no_templates()}
				</h4>
				{#if !isReadonly}
					<p class="text-muted-content text-sm font-light">
						{m.hosted_agents_templates_no_templates_desc()}
					</p>
					<button
						class="btn btn-primary flex items-center gap-1 text-sm"
						onclick={() => goto(`${page.url.pathname}?view=templates&new=true`)}
					>
						<Plus class="size-4" />
						{m.hosted_agents_templates_add_template()}
					</button>
				{/if}
			</div>
		{:else}
			<Table
				data={tableData}
				fields={['name', 'harness']}
				headers={[
					{ property: 'name', title: m.core_name() },
					{ property: 'harness', title: m.chat_templates_col_harness() }
				]}
				onClickRow={(d, isCtrlClick) => {
					openUrl(`/hosted-agents/${d.id}`, isCtrlClick);
				}}
				sortable={['name', 'harness']}
			>
				{#snippet onRenderColumn(property, d)}
					{#if property === 'name'}
						<div class="flex items-center gap-2">
							<AgentIcon icon={d.icon} iconDark={d.iconDark} alt="" />
							<span class="truncate">{d.name}</span>
						</div>
					{:else}
						{d[property as keyof typeof d]}
					{/if}
				{/snippet}
				{#snippet actions(d)}
					{#if !isReadonly}
						<IconButton
							variant="danger"
							onclick={(e) => {
								e.stopPropagation();
								agentToDelete = hostedAgents.find((a) => a.id === d.id);
							}}
							tooltip={{ text: m.hosted_agents_templates_delete_agent() }}
						>
							<Trash2 class="size-4" />
						</IconButton>
					{/if}
				{/snippet}
			</Table>
		{/if}
	</div>
{/if}

<Confirm
	msg={m.chat_delete_named({ name: agentToDelete?.name || m.chat_templates_this_agent() })}
	show={Boolean(agentToDelete)}
	onsuccess={async () => {
		if (!agentToDelete) return;
		await AdminService.deleteHostedAgent(agentToDelete.id);
		hostedAgents = await AdminService.listHostedAgents({ all: true });
		agentToDelete = undefined;
	}}
	oncancel={() => (agentToDelete = undefined)}
/>
