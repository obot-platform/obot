<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { AdminService, UserService, type OrgUser } from '$lib/services';
	import type {
		HostedAgentPool,
		HostedAgentPoolAssignment,
		HostedAgentPoolDefaults,
		HostedAgentPoolUtilization
	} from '$lib/services/admin/types';
	import { errors } from '$lib/stores';
	import { Activity, Pencil, Plus, Trash2 } from '@lucide/svelte';
	import { onMount } from 'svelte';

	interface Props {
		pools: HostedAgentPool[];
		defaults?: HostedAgentPoolDefaults;
		assignments: HostedAgentPoolAssignment[];
		readonly?: boolean;
	}

	let {
		pools = $bindable(),
		defaults = $bindable(),
		assignments = $bindable(),
		readonly = false
	}: Props = $props();

	let poolDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let assignmentDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let usageDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let editing = $state<HostedAgentPool>();
	let deleting = $state<HostedAgentPool>();
	let deletingAssignment = $state<HostedAgentPoolAssignment>();
	let saving = $state(false);
	let usage = $state<HostedAgentPoolUtilization>();
	let usagePool = $state<HostedAgentPool>();
	let users = $state<OrgUser[]>([]);
	let usersByID = $derived(new Map(users.map((u) => [u.id, u])));

	// A pool's ID says nothing an administrator can act on. Its members do, so
	// resolve them to people and let the pool be recognised by who is in it.
	function userLabel(id: string) {
		const user = usersByID.get(id);
		if (!user) return m.hosted_agents_pools_user_id_label({ id });
		return (
			user.displayName || user.email || user.username || m.hosted_agents_pools_user_id_label({ id })
		);
	}

	function membersOf(poolID: string) {
		return assignments.filter((a) => a.poolID === poolID);
	}

	let form = $state({ cpu: 1, memory: 4, storage: 20, maxSandboxes: 10, suspended: false });
	let assignmentForm = $state({ userID: '', poolID: '', default: true });
	let defaultsForm = $state({ cpu: 1, memory: 4, storage: 20, maxSandboxes: 10 });
	const fastPollInterval = 2_000;
	const slowPollInterval = 30_000;
	let pollTimer: ReturnType<typeof setTimeout> | undefined;
	let destroyed = false;

	$effect(() => {
		if (defaults) {
			defaultsForm = {
				cpu: defaults.capacity.cpuVcpus,
				memory: toGiB(defaults.capacity.memoryBytes),
				storage: toGiB(defaults.capacity.storageBytes),
				maxSandboxes: defaults.maxSandboxes ?? 10
			};
		}
	});

	const toGiB = (bytes: number) => Math.round((bytes / 1024 ** 3) * 10) / 10;
	const quantity = (cpu: number, memory: number, storage: number) => ({
		cpuVcpus: Number(cpu),
		memoryBytes: Math.round(Number(memory) * 1024 ** 3),
		storageBytes: Math.round(Number(storage) * 1024 ** 3)
	});
	const formatQuantity = (q?: { cpuVcpus: number; memoryBytes: number; storageBytes: number }) =>
		q
			? m.hosted_agents_pools_quantity({
					cpu: q.cpuVcpus,
					memory: toGiB(q.memoryBytes),
					storage: toGiB(q.storageBytes)
				})
			: '—';

	async function reload() {
		[pools, assignments] = await Promise.all([
			AdminService.listHostedAgentPools(),
			AdminService.listHostedAgentPoolAssignments()
		]);
	}

	onMount(async () => {
		try {
			users = await UserService.listUsers();
		} catch {
			// Names are a nicety; without them members still render by ID.
		}
	});

	function hasTransitionalPools() {
		return pools.some(
			(pool) =>
				Boolean(pool.deleted) ||
				(!pool.status?.observedRevision && !pool.status?.ready && !pool.status?.degraded)
		);
	}

	function scheduleRefresh(delay = hasTransitionalPools() ? fastPollInterval : slowPollInterval) {
		clearTimeout(pollTimer);
		if (destroyed) return;
		pollTimer = setTimeout(poll, delay);
	}

	async function poll() {
		if (document.visibilityState === 'hidden') {
			scheduleRefresh(slowPollInterval);
			return;
		}
		try {
			await reload();
			if (usagePool) {
				usage = await AdminService.getHostedAgentPoolUtilization(usagePool.id);
			}
		} catch {
			// Background status observation retries without producing recurring
			// administrator notifications.
		}
		scheduleRefresh();
	}

	function openPool(value?: HostedAgentPool) {
		editing = value;
		form = value
			? {
					cpu: value.capacity.cpuVcpus,
					memory: toGiB(value.capacity.memoryBytes),
					storage: toGiB(value.capacity.storageBytes),
					maxSandboxes: value.maxSandboxes ?? 10,
					suspended: Boolean(value.suspended)
				}
			: { cpu: 1, memory: 4, storage: 20, maxSandboxes: 10, suspended: false };
		poolDialog?.open();
	}

	async function savePool() {
		saving = true;
		try {
			const manifest = {
				capacity: quantity(form.cpu, form.memory, form.storage),
				maxSandboxes: Number(form.maxSandboxes),
				suspended: form.suspended
			};
			if (editing) await AdminService.updateHostedAgentPool(editing.id, manifest);
			else await AdminService.createHostedAgentPool(manifest);
			await reload();
			scheduleRefresh();
			poolDialog?.close();
		} catch (error) {
			errors.append(m.hosted_agents_pools_save_failed({ error: String(error) }));
		} finally {
			saving = false;
		}
	}

	async function saveDefaults() {
		saving = true;
		try {
			const manifest = {
				capacity: quantity(defaultsForm.cpu, defaultsForm.memory, defaultsForm.storage),
				maxSandboxes: Number(defaultsForm.maxSandboxes)
			};
			defaults = defaults
				? await AdminService.updateHostedAgentPoolDefaults(manifest)
				: await AdminService.createHostedAgentPoolDefaults(manifest);
		} catch (error) {
			errors.append(m.hosted_agents_pools_save_defaults_failed({ error: String(error) }));
		} finally {
			saving = false;
		}
	}

	async function saveAssignment() {
		saving = true;
		try {
			await AdminService.createHostedAgentPoolAssignment(assignmentForm);
			await reload();
			scheduleRefresh();
			assignmentDialog?.close();
		} catch (error) {
			errors.append(m.hosted_agents_pools_assign_failed({ error: String(error) }));
		} finally {
			saving = false;
		}
	}

	async function showUsage(pool: HostedAgentPool) {
		usagePool = pool;
		usage = undefined;
		usageDialog?.open();
		try {
			usage = await AdminService.getHostedAgentPoolUtilization(pool.id);
		} catch (error) {
			errors.append(m.hosted_agents_pools_load_utilization_failed({ error: String(error) }));
		}
	}

	onMount(() => {
		const handleVisibilityChange = () => {
			if (document.visibilityState === 'visible') scheduleRefresh(0);
			else scheduleRefresh(slowPollInterval);
		};
		document.addEventListener('visibilitychange', handleVisibilityChange);
		scheduleRefresh(0);
		return () => {
			destroyed = true;
			clearTimeout(pollTimer);
			document.removeEventListener('visibilitychange', handleVisibilityChange);
		};
	});
</script>

<div class="flex flex-col gap-6">
	<section class="dark:bg-base-300 border-base-400 rounded-lg border bg-white p-4 shadow-sm">
		<!-- Four small numbers and a button on one row: they are read together and
		     each is a couple of characters wide, so stacking them wasted the row. -->
		<div class="flex flex-wrap items-end gap-x-4 gap-y-3">
			<div class="mr-auto">
				<h2 class="font-semibold">{m.hosted_agents_pools_deployment_defaults()}</h2>
				<p class="text-muted-content text-xs">{m.hosted_agents_pools_deployment_defaults_hint()}</p>
			</div>
			<label class="text-muted-content flex flex-col text-xs"
				>vCPU<input
					type="number"
					min="0.1"
					step="0.1"
					class="text-input-filled mt-0.5 w-20"
					disabled={readonly}
					bind:value={defaultsForm.cpu}
				/></label
			>
			<label class="text-muted-content flex flex-col text-xs"
				>{m.hosted_agents_pools_memory_gib()}<input
					type="number"
					min="0.1"
					step="0.1"
					class="text-input-filled mt-0.5 w-20"
					disabled={readonly}
					bind:value={defaultsForm.memory}
				/></label
			>
			<label class="text-muted-content flex flex-col text-xs"
				>{m.hosted_agents_pools_storage_gib()}<input
					type="number"
					min="0.1"
					step="0.1"
					class="text-input-filled mt-0.5 w-20"
					disabled={readonly}
					bind:value={defaultsForm.storage}
				/></label
			>
			<label class="text-muted-content flex flex-col text-xs"
				>{m.hosted_agents_pools_max_sandboxes()}<input
					type="number"
					min="1"
					step="1"
					class="text-input-filled mt-0.5 w-20"
					disabled={readonly}
					bind:value={defaultsForm.maxSandboxes}
				/></label
			>
			{#if !readonly}
				<button
					class="btn btn-primary text-sm"
					disabled={saving || !defaultsForm.cpu || !defaultsForm.memory || !defaultsForm.storage}
					onclick={saveDefaults}>{m.core_save()}</button
				>
			{/if}
		</div>
	</section>

	<section>
		<div class="mb-3 flex items-center justify-between">
			<div>
				<h2 class="font-semibold">{m.hosted_agents_pools()}</h2>
				<p class="text-muted-content text-sm">
					{m.hosted_agents_pools_hint()}
				</p>
			</div>
			<div class="flex gap-2">
				{#if !readonly}
					<button class="btn btn-secondary text-sm" onclick={() => assignmentDialog?.open()}
						><Plus class="size-4" /> {m.hosted_agents_pools_assign_user()}</button
					>
					<button class="btn btn-primary text-sm" onclick={() => openPool()}
						><Plus class="size-4" /> {m.hosted_agents_pools_add_pool()}</button
					>
				{/if}
			</div>
		</div>
		<div class="grid gap-3">
			{#each pools as pool (pool.id)}
				{@const members = membersOf(pool.id)}
				<div class="dark:bg-base-300 border-base-400 rounded-lg border bg-white p-4 shadow-sm">
					<div class="flex justify-between gap-4">
						<div class="min-w-0">
							<div class="flex flex-wrap items-center gap-2">
								<!-- A pool is recognised by who is in it, not by its identifier. -->
								<span class="font-medium">
									{#if members.length === 1}
										{userLabel(members[0].userID)}
									{:else if members.length > 1}
										{userLabel(members[0].userID)} +{members.length - 1}
									{:else}
										{m.hosted_agents_pools_unassigned()}
									{/if}
								</span>
								<span
									class="badge badge-sm {pool.suspended
										? 'badge-warning'
										: pool.status?.ready
											? 'badge-success'
											: 'badge-secondary'}"
									>{pool.suspended
										? m.hosted_agents_suspended()
										: pool.status?.ready
											? m.core_status_ready()
											: m.core_status_pending()}</span
								>
							</div>
							<p class="text-muted-content mt-1 text-sm">
								{m.hosted_agents_pools_capacity_summary({
									quantity: formatQuantity(pool.capacity),
									count: pool.maxSandboxes ?? 10
								})}
							</p>
							{#if pool.status?.message}<p class="text-warning mt-1 text-xs">
									{pool.status.message}
								</p>{/if}
							<p class="text-muted-content mt-1 font-mono text-xs opacity-60">{pool.id}</p>
						</div>
						<div class="flex shrink-0">
							<button class="btn btn-ghost btn-sm" onclick={() => showUsage(pool)}
								><Activity class="size-4" /> {m.hosted_agents_pools_usage()}</button
							>
							{#if !readonly}
								<button
									class="btn btn-ghost btn-sm"
									aria-label={m.hosted_agents_pools_edit_pool()}
									onclick={() => openPool(pool)}><Pencil class="size-4" /></button
								>
								<button
									class="btn btn-ghost btn-sm text-error"
									aria-label={m.hosted_agents_pools_delete_pool()}
									onclick={() => (deleting = pool)}><Trash2 class="size-4" /></button
								>
							{/if}
						</div>
					</div>

					<!-- Members inline, so a pool and its people are one thing rather than
					     two lists cross-referenced by ID. -->
					<div class="border-base-400 mt-3 flex flex-wrap items-center gap-2 border-t pt-3">
						{#each members as member (member.id)}
							<span
								class="border-base-400 flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs"
							>
								{userLabel(member.userID)}
								{#if member.default}<span class="text-muted-content"
										>{m.hosted_agents_pools_member_default()}</span
									>{/if}
								{#if !readonly}
									<button
										class="text-muted-content hover:text-error"
										aria-label={m.hosted_agents_pools_remove_member({
											name: userLabel(member.userID)
										})}
										onclick={() => (deletingAssignment = member)}><Trash2 class="size-3" /></button
									>
								{/if}
							</span>
						{:else}
							<span class="text-muted-content text-xs">{m.hosted_agents_pools_no_users()}</span>
						{/each}
					</div>
				</div>
			{:else}
				<p class="text-muted-content py-6 text-center text-sm">
					{m.hosted_agents_pools_no_pools()}
				</p>
			{/each}
		</div>
	</section>
</div>

<ResponsiveDialog
	bind:this={poolDialog}
	title={editing ? m.hosted_agents_pools_edit_pool() : m.hosted_agents_pools_add_pool()}
	class="md:max-w-md"
>
	<div class="grid gap-3">
		<label class="text-sm"
			>vCPU<input
				type="number"
				min="0.1"
				step="0.1"
				class="text-input-filled mt-1"
				bind:value={form.cpu}
			/></label
		>
		<label class="text-sm"
			>{m.hosted_agents_pools_memory_gib()}<input
				type="number"
				min="0.1"
				step="0.1"
				class="text-input-filled mt-1"
				bind:value={form.memory}
			/></label
		>
		<label class="text-sm"
			>{m.hosted_agents_pools_storage_gib()}<input
				type="number"
				min="0.1"
				step="0.1"
				class="text-input-filled mt-1"
				bind:value={form.storage}
			/></label
		>
		<label class="text-sm"
			>{m.hosted_agents_pools_max_sandboxes()}<input
				type="number"
				min="1"
				step="1"
				class="text-input-filled mt-1"
				bind:value={form.maxSandboxes}
			/><span class="text-muted-content mt-1 block text-xs">
				{m.hosted_agents_pools_sandboxes_hint()}
			</span></label
		>
		<label class="flex items-center gap-2 text-sm"
			><input type="checkbox" bind:checked={form.suspended} />
			{m.hosted_agents_pools_suspend_new_starts()}</label
		>
		<button
			class="btn btn-primary mt-2"
			disabled={saving || !form.cpu || !form.memory || !form.storage}
			onclick={savePool}
			>{#if saving}<Loading class="size-4" />{:else}{m.core_save()}{/if}</button
		>
	</div>
</ResponsiveDialog>

<ResponsiveDialog
	bind:this={assignmentDialog}
	title={m.hosted_agents_pools_assign_pool()}
	class="md:max-w-md"
>
	<div class="grid gap-3">
		<label class="text-sm"
			>{m.hosted_agents_pools_user_id()}<input
				class="text-input-filled mt-1"
				bind:value={assignmentForm.userID}
			/></label
		>
		<label class="text-sm"
			>{m.hosted_agents_pools_pool()}<select
				class="text-input-filled mt-1"
				bind:value={assignmentForm.poolID}
				><option value="">{m.hosted_agents_pools_select_pool()}</option
				>{#each pools as pool (pool.id)}<option value={pool.id}>{pool.id}</option>{/each}</select
			></label
		>
		<label class="flex items-center gap-2 text-sm"
			><input type="checkbox" bind:checked={assignmentForm.default} />
			{m.hosted_agents_pools_default_pool()}</label
		>
		<button
			class="btn btn-primary"
			disabled={saving || !assignmentForm.userID || !assignmentForm.poolID}
			onclick={saveAssignment}>{m.hosted_agents_pools_assign()}</button
		>
	</div>
</ResponsiveDialog>

<ResponsiveDialog
	bind:this={usageDialog}
	title={m.hosted_agents_pools_live_utilization()}
	class="md:max-w-lg"
>
	{#if !usage}<div class="flex justify-center p-8"><Loading class="size-6" /></div>
	{:else}
		<div class="grid gap-3">
			<p class="text-sm">
				<strong>{usagePool?.id}</strong>
				{m.hosted_agents_pools_snapshot({ time: new Date(usage.timestamp).toLocaleString() })}
			</p>
			<p class="text-sm">{m.hosted_agents_pools_used({ quantity: formatQuantity(usage.pool) })}</p>
			<p class="text-muted-content text-xs">
				{m.hosted_agents_pools_pressure({
					cpu: usage.pressure.cpu ?? m.hosted_agents_pools_unknown(),
					memory: usage.pressure.memory ?? m.hosted_agents_pools_unknown(),
					storage: usage.pressure.storage ?? m.hosted_agents_pools_unknown()
				})}
			</p>
			<p class="text-muted-content text-xs">
				{m.hosted_agents_pools_totals_hint()}
			</p>
			{#each usage.instances as instance (instance.instanceID)}<div
					class="border-base-400 rounded border p-2 text-xs"
				>
					{instance.instanceID} · {instance.state ?? m.hosted_agents_pools_unknown()} · {formatQuantity(
						instance.usage
					)}
				</div>{/each}
		</div>
	{/if}
</ResponsiveDialog>

{#if deleting}
	<Confirm
		msg={m.hosted_agents_pools_delete_confirm()}
		show
		onsuccess={async () => {
			await AdminService.deleteHostedAgentPool(deleting!.id);
			deleting = undefined;
			await reload();
			scheduleRefresh();
		}}
		oncancel={() => (deleting = undefined)}
	/>
{/if}
{#if deletingAssignment}
	<Confirm
		msg={m.hosted_agents_pools_remove_assignment_confirm()}
		show
		onsuccess={async () => {
			await AdminService.deleteHostedAgentPoolAssignment(deletingAssignment!.id);
			deletingAssignment = undefined;
			await reload();
			scheduleRefresh();
		}}
		oncancel={() => (deletingAssignment = undefined)}
	/>
{/if}
