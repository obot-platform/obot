<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import HostedAgentInstanceForm from '$lib/components/hosted-agents/HostedAgentInstanceForm.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import Loading from '$lib/icons/Loading.svelte';
	import { AdminService, type HostedAgentInstance } from '$lib/services';
	import { errors } from '$lib/stores';
	import { ExternalLink, Pencil, Plus, Trash2 } from '@lucide/svelte';
	import { onDestroy, untrack } from 'svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	const { hostedAgent } = $derived(data);
	let instances = $state(untrack(() => data.instances));

	let instanceToDelete = $state<HostedAgentInstance>();
	let formDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let saving = $state(false);
	let editing = $state<HostedAgentInstance | undefined>();
	let form = $state({
		name: '',
		description: '',
		icon: '',
		answers: {} as Record<string, string>,
		gitRepo: '',
		mcpServers: [] as string[],
		skills: [] as string[],
		models: [] as string[]
	});

	// Seed answers from the agent's defaults so the dialog shows what will be sent.
	function defaultAnswers(): Record<string, string> {
		const result: Record<string, string> = {};
		for (const question of hostedAgent?.questions ?? []) {
			if (question.default) result[question.key] = question.default;
			else if (question.type === 'boolean') result[question.key] = 'false';
		}
		return result;
	}

	const duration = PAGE_TRANSITION_DURATION;

	let max = $derived(hostedAgent?.maxInstancesPerUser ?? 0);
	let atQuota = $derived(max > 0 && instances.length >= max);

	// Mirror the server's required-answer rule so the button reflects it. The
	// server remains the authority; this is only to avoid a pointless round trip.
	let canSave = $derived(
		Boolean(form.name) &&
			(hostedAgent?.questions ?? []).every(
				(q) => !q.required || q.default || (form.answers[q.key] ?? '') !== ''
			)
	);

	let tableData = $derived(
		instances.map((instance) => ({
			id: instance.id,
			name: instance.name,
			description: instance.description ?? '',
			state: instance.status?.state ?? 'pending',
			url: instance.status?.url ?? ''
		}))
	);

	async function refresh() {
		if (!hostedAgent) return;
		instances = await AdminService.listHostedAgentInstances(hostedAgent.id);
	}

	// An instance is created before the controller has assigned it a URL, so poll
	// until everything settles rather than leaving the row stuck on "pending".
	let pollTimer: ReturnType<typeof setTimeout> | undefined;
	let pollAttempts = 0;

	function stopPolling() {
		clearTimeout(pollTimer);
		pollTimer = undefined;
		pollAttempts = 0;
	}

	function pollUntilSettled() {
		stopPolling();
		const tick = async () => {
			pollAttempts++;
			try {
				await refresh();
			} catch {
				stopPolling();
				return;
			}
			const settled = instances.every((i) => i.status?.state && i.status.state !== 'pending');
			if (settled || pollAttempts >= 15) {
				stopPolling();
				return;
			}
			pollTimer = setTimeout(tick, 1000);
		};
		pollTimer = setTimeout(tick, 500);
	}

	$effect(() => {
		if (instances.some((i) => !i.status?.state || i.status.state === 'pending') && !pollTimer) {
			pollUntilSettled();
		}
	});

	onDestroy(stopPolling);

	function openCreate() {
		editing = undefined;
		form = {
			name: '',
			description: '',
			icon: '',
			answers: defaultAnswers(),
			gitRepo: '',
			mcpServers: [],
			skills: [],
			models: []
		};
		formDialog?.open();
	}

	function openEdit(instance: HostedAgentInstance) {
		editing = instance;
		form = {
			name: instance.name,
			description: instance.description ?? '',
			icon: instance.icon ?? '',
			answers: { ...defaultAnswers(), ...(instance.answers ?? {}) },
			gitRepo: instance.gitRepo ?? '',
			mcpServers: [...(instance.mcpServers ?? [])],
			skills: [...(instance.skills ?? [])],
			models: [...(instance.models ?? [])]
		};
		formDialog?.open();
	}

	async function save() {
		if (!hostedAgent) return;
		saving = true;
		try {
			const manifest = {
				name: form.name,
				description: form.description,
				icon: form.icon,
				answers: form.answers,
				gitRepo: form.gitRepo,
				mcpServers: form.mcpServers,
				skills: form.skills,
				models: form.models
			};
			if (editing) {
				await AdminService.updateHostedAgentInstance(editing.id, manifest);
			} else {
				await AdminService.createHostedAgentInstance({
					hostedAgentID: hostedAgent.id,
					...manifest
				});
			}
			await refresh();
			pollUntilSettled();
			formDialog?.close();
		} catch (error) {
			errors.append(`Failed to ${editing ? 'update' : 'create'} instance: ${error}`);
		} finally {
			saving = false;
		}
	}
</script>

<Layout title={hostedAgent?.name ?? 'Agent'} showBackButton>
	<div
		class="flex h-full w-full flex-col gap-4"
		in:fly={{ x: 100, duration }}
		out:fly={{ x: -100, duration }}
	>
		{#if hostedAgent?.description}
			<p class="text-muted-content text-sm font-light">{hostedAgent.description}</p>
		{/if}

		<div class="flex items-center justify-between">
			<h2 class="text-lg font-semibold">Your Instances</h2>
			<button
				class="btn btn-primary flex items-center gap-1 text-sm"
				disabled={atQuota}
				onclick={openCreate}
			>
				<Plus class="size-4" /> New Instance
			</button>
		</div>

		{#if atQuota}
			<p class="text-muted-content text-xs">
				You've reached the limit of {max} instance{max === 1 ? '' : 's'} for this agent. Delete one to
				create another.
			</p>
		{/if}

		<Table
			data={tableData}
			fields={['name', 'description', 'state', 'url']}
			headers={[
				{ property: 'name', title: 'Name' },
				{ property: 'description', title: 'Description' },
				{ property: 'state', title: 'State' },
				{ property: 'url', title: 'URL' }
			]}
			noDataMessage="No instances yet."
		>
			{#snippet onRenderColumn(property, d)}
				{#if property === 'state'}
					<span
						class="badge badge-sm {d.state === 'ready'
							? 'badge-success'
							: d.state === 'error'
								? 'badge-error'
								: 'badge-secondary'}"
					>
						{d.state}
					</span>
				{:else if property === 'url'}
					{#if d.url}
						<a
							href={d.url}
							target="_blank"
							rel="external noopener noreferrer"
							class="link flex items-center gap-1"
						>
							<ExternalLink class="size-4" /> Open
						</a>
					{:else}
						<span class="text-muted-content">-</span>
					{/if}
				{:else}
					{d[property as keyof typeof d]}
				{/if}
			{/snippet}
			{#snippet actions(d)}
				<IconButton
					onclick={(e) => {
						e.stopPropagation();
						const instance = instances.find((i) => i.id === d.id);
						if (instance) openEdit(instance);
					}}
					tooltip={{ text: 'Edit Instance' }}
				>
					<Pencil class="size-4" />
				</IconButton>
				<IconButton
					variant="danger"
					onclick={(e) => {
						e.stopPropagation();
						instanceToDelete = instances.find((i) => i.id === d.id);
					}}
					tooltip={{ text: 'Delete Instance' }}
				>
					<Trash2 class="size-4" />
				</IconButton>
			{/snippet}
		</Table>
	</div>
</Layout>

<ResponsiveDialog
	bind:this={formDialog}
	title={editing ? 'Edit Instance' : 'New Instance'}
	class="default-scrollbar-thin max-h-[85vh] overflow-y-auto md:max-w-md"
>
	{#if hostedAgent}
		<HostedAgentInstanceForm
			agent={hostedAgent}
			bind:name={form.name}
			bind:description={form.description}
			bind:icon={form.icon}
			bind:answers={form.answers}
			bind:gitRepo={form.gitRepo}
			bind:mcpServers={form.mcpServers}
			bind:skills={form.skills}
			bind:models={form.models}
		/>
	{/if}
	<div class="flex justify-end gap-2 pt-4">
		<button class="btn btn-secondary text-sm" onclick={() => formDialog?.close()}>Cancel</button>
		<button class="btn btn-primary text-sm" disabled={!canSave || saving} onclick={save}>
			{#if saving}
				<Loading class="size-4" />
			{:else}
				{editing ? 'Update' : 'Create'}
			{/if}
		</button>
	</div>
</ResponsiveDialog>

<Confirm
	msg={`Delete ${instanceToDelete?.name || 'this instance'}?`}
	show={Boolean(instanceToDelete)}
	onsuccess={async () => {
		if (!instanceToDelete) return;
		await AdminService.deleteHostedAgentInstance(instanceToDelete.id);
		await refresh();
		instanceToDelete = undefined;
	}}
	oncancel={() => (instanceToDelete = undefined)}
/>

<svelte:head>
	<title>Obot | {hostedAgent?.name ?? 'Agent'}</title>
</svelte:head>
