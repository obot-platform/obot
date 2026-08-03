<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
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
		if (!user) return `User ${id}`;
		return user.displayName || user.email || user.username || `User ${id}`;
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
			? `${q.cpuVcpus} vCPU · ${toGiB(q.memoryBytes)} GiB RAM · ${toGiB(q.storageBytes)} GiB disk`
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
			errors.append(`Failed to save pool: ${error}`);
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
			errors.append(`Failed to save pool defaults: ${error}`);
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
			errors.append(`Failed to assign pool: ${error}`);
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
			errors.append(`Failed to load utilization: ${error}`);
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
				<h2 class="font-semibold">Deployment defaults</h2>
				<p class="text-muted-content text-xs">Applied when a pool is created on demand.</p>
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
				>Memory (GiB)<input
					type="number"
					min="0.1"
					step="0.1"
					class="text-input-filled mt-0.5 w-20"
					disabled={readonly}
					bind:value={defaultsForm.memory}
				/></label
			>
			<label class="text-muted-content flex flex-col text-xs"
				>Storage (GiB)<input
					type="number"
					min="0.1"
					step="0.1"
					class="text-input-filled mt-0.5 w-20"
					disabled={readonly}
					bind:value={defaultsForm.storage}
				/></label
			>
			<label class="text-muted-content flex flex-col text-xs"
				>Max sandboxes<input
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
					onclick={saveDefaults}>Save</button
				>
			{/if}
		</div>
	</section>

	<section>
		<div class="mb-3 flex items-center justify-between">
			<div>
				<h2 class="font-semibold">Pools</h2>
				<p class="text-muted-content text-sm">
					Shared CPU, memory and disk. Every member's sandboxes draw from the same pool.
				</p>
			</div>
			<div class="flex gap-2">
				{#if !readonly}
					<button class="btn btn-secondary text-sm" onclick={() => assignmentDialog?.open()}
						><Plus class="size-4" /> Assign user</button
					>
					<button class="btn btn-primary text-sm" onclick={() => openPool()}
						><Plus class="size-4" /> Add pool</button
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
										Unassigned pool
									{/if}
								</span>
								<span
									class="badge badge-sm {pool.suspended
										? 'badge-warning'
										: pool.status?.ready
											? 'badge-success'
											: 'badge-secondary'}"
									>{pool.suspended ? 'Suspended' : pool.status?.ready ? 'Ready' : 'Pending'}</span
								>
							</div>
							<p class="text-muted-content mt-1 text-sm">
								{formatQuantity(pool.capacity)} · up to {pool.maxSandboxes ?? 10} sandboxes
							</p>
							{#if pool.status?.message}<p class="text-warning mt-1 text-xs">
									{pool.status.message}
								</p>{/if}
							<p class="text-muted-content mt-1 font-mono text-xs opacity-60">{pool.id}</p>
						</div>
						<div class="flex shrink-0">
							<button class="btn btn-ghost btn-sm" onclick={() => showUsage(pool)}
								><Activity class="size-4" /> Usage</button
							>
							{#if !readonly}
								<button
									class="btn btn-ghost btn-sm"
									aria-label="Edit pool"
									onclick={() => openPool(pool)}><Pencil class="size-4" /></button
								>
								<button
									class="btn btn-ghost btn-sm text-error"
									aria-label="Delete pool"
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
								{#if member.default}<span class="text-muted-content">· default</span>{/if}
								{#if !readonly}
									<button
										class="text-muted-content hover:text-error"
										aria-label="Remove {userLabel(member.userID)} from pool"
										onclick={() => (deletingAssignment = member)}><Trash2 class="size-3" /></button
									>
								{/if}
							</span>
						{:else}
							<span class="text-muted-content text-xs">No users assigned.</span>
						{/each}
					</div>
				</div>
			{:else}
				<p class="text-muted-content py-6 text-center text-sm">No pools yet.</p>
			{/each}
		</div>
	</section>
</div>

<ResponsiveDialog
	bind:this={poolDialog}
	title={editing ? 'Edit pool' : 'Add pool'}
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
			>Memory (GiB)<input
				type="number"
				min="0.1"
				step="0.1"
				class="text-input-filled mt-1"
				bind:value={form.memory}
			/></label
		>
		<label class="text-sm"
			>Storage (GiB)<input
				type="number"
				min="0.1"
				step="0.1"
				class="text-input-filled mt-1"
				bind:value={form.storage}
			/></label
		>
		<label class="text-sm"
			>Max sandboxes<input
				type="number"
				min="1"
				step="1"
				class="text-input-filled mt-1"
				bind:value={form.maxSandboxes}
			/><span class="text-muted-content mt-1 block text-xs">
				Sandboxes share this pool. Each is guaranteed capacity ÷ this number and may burst to the
				whole pool, so a higher number means smaller guaranteed shares.
			</span></label
		>
		<label class="flex items-center gap-2 text-sm"
			><input type="checkbox" bind:checked={form.suspended} /> Suspend new starts</label
		>
		<button
			class="btn btn-primary mt-2"
			disabled={saving || !form.cpu || !form.memory || !form.storage}
			onclick={savePool}
			>{#if saving}<Loading class="size-4" />{:else}Save{/if}</button
		>
	</div>
</ResponsiveDialog>

<ResponsiveDialog bind:this={assignmentDialog} title="Assign pool" class="md:max-w-md">
	<div class="grid gap-3">
		<label class="text-sm"
			>User ID<input class="text-input-filled mt-1" bind:value={assignmentForm.userID} /></label
		>
		<label class="text-sm"
			>Pool<select class="text-input-filled mt-1" bind:value={assignmentForm.poolID}
				><option value="">Select a pool</option>{#each pools as pool (pool.id)}<option
						value={pool.id}>{pool.id}</option
					>{/each}</select
			></label
		>
		<label class="flex items-center gap-2 text-sm"
			><input type="checkbox" bind:checked={assignmentForm.default} /> Default pool</label
		>
		<button
			class="btn btn-primary"
			disabled={saving || !assignmentForm.userID || !assignmentForm.poolID}
			onclick={saveAssignment}>Assign</button
		>
	</div>
</ResponsiveDialog>

<ResponsiveDialog bind:this={usageDialog} title="Live utilization" class="md:max-w-lg">
	{#if !usage}<div class="flex justify-center p-8"><Loading class="size-6" /></div>
	{:else}
		<div class="grid gap-3">
			<p class="text-sm">
				<strong>{usagePool?.id}</strong> · snapshot {new Date(usage.timestamp).toLocaleString()}
			</p>
			<p class="text-sm">Used: {formatQuantity(usage.pool)}</p>
			<p class="text-muted-content text-xs">
				Pressure: CPU {usage.pressure.cpu ?? 'unknown'} · memory {usage.pressure.memory ??
					'unknown'} · storage {usage.pressure.storage ?? 'unknown'}
			</p>
			<p class="text-muted-content text-xs">
				Totals cover every member of this pool. Disk is not reported per instance, because members
				share one volume.
			</p>
			{#each usage.instances as instance (instance.instanceID)}<div
					class="border-base-400 rounded border p-2 text-xs"
				>
					{instance.instanceID} · {instance.state ?? 'unknown'} · {formatQuantity(instance.usage)}
				</div>{/each}
		</div>
	{/if}
</ResponsiveDialog>

{#if deleting}
	<Confirm
		msg="Delete this pool? Deletion completes only after the backend resources are gone."
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
		msg="Remove this assignment? The user will no longer be able to select the pool."
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
