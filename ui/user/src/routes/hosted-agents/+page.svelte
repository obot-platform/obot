<script lang="ts">
	import { resolve } from '$app/paths';
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import AgentIcon from '$lib/components/hosted-agents/AgentIcon.svelte';
	import HostedAgentInstanceForm from '$lib/components/hosted-agents/HostedAgentInstanceForm.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { parseErrorContent } from '$lib/errors';
	import Loading from '$lib/icons/Loading.svelte';
	import { AdminService } from '$lib/services';
	import type {
		HostedAgent,
		HostedAgentPool,
		HostedAgentPoolUtilization,
		HostedAgentInstance,
		HostedAgentResourceQuantity
	} from '$lib/services/admin/types';
	import {
		Bot,
		ChevronDown,
		ChevronRight,
		ExternalLink,
		Pencil,
		Plus,
		SquareTerminal,
		Trash2
	} from '@lucide/svelte';
	import { onMount, untrack } from 'svelte';
	import { SvelteMap, SvelteSet } from 'svelte/reactivity';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	let hostedAgents = $state<HostedAgent[]>(untrack(() => data.hostedAgents));
	let instances = $state<HostedAgentInstance[]>(untrack(() => data.instances));
	let pools = $state<HostedAgentPool[]>(untrack(() => data.pools));
	let expanded = new SvelteSet<string>();
	let utilization = new SvelteMap<string, HostedAgentPoolUtilization>();
	let formDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let editing = $state<HostedAgentInstance>();
	let selectedAgent = $state<HostedAgent>();
	let selectedPoolID = $state('');
	let deleting = $state<HostedAgentInstance>();
	let saving = $state(false);
	let formError = $state('');
	let form = $state(emptyForm());

	const duration = PAGE_TRANSITION_DURATION;
	const gibibyte = 1024 ** 3;
	const fastPollInterval = 2_000;
	const slowPollInterval = 30_000;
	let pollTimer: ReturnType<typeof setTimeout> | undefined;
	let destroyed = false;
	let displayedPools = $derived(
		pools.length
			? pools
			: [
					{
						id: '',
						created: '',
						capacity: { cpuVcpus: 0, memoryBytes: 0, storageBytes: 0 }
					} satisfies HostedAgentPool
				]
	);

	function emptyForm() {
		return {
			name: '',
			description: '',
			icon: '',
			answers: {} as Record<string, string>,
			gitRepo: '',
			gitRef: '',
			mcpServers: [] as string[],
			skills: [] as string[],
			models: [] as string[]
		};
	}

	function defaultAnswers(agent: HostedAgent) {
		return Object.fromEntries(
			(agent.questions ?? []).map((question) => [
				question.key,
				question.default ?? (question.type === 'boolean' ? 'false' : '')
			])
		);
	}

	function instancesFor(agentID: string, poolID: string) {
		return instances.filter(
			(instance) => instance.hostedAgentID === agentID && (instance.poolID ?? '') === poolID
		);
	}

	// An agent naming a model or MCP server this installation does not have
	// cannot produce a working sandbox, so it is offered but not launchable —
	// visible, with the reason, rather than silently missing.
	function unavailableReason(agent: HostedAgent) {
		return (agent.unavailableReasons ?? []).join('; ');
	}

	function isAtInstanceLimit(agent: HostedAgent) {
		const maximum = agent.maxInstancesPerUser ?? 0;
		return (
			maximum > 0 &&
			instances.filter((instance) => instance.hostedAgentID === agent.id).length >= maximum
		);
	}

	function percent(used: number, capacity: number) {
		return capacity > 0 ? Math.min(100, Math.round((used / capacity) * 100)) : 0;
	}

	function usageForInstance(poolID: string, instanceID: string) {
		return utilization.get(poolID)?.instances.find((item) => item.instanceID === instanceID)?.usage;
	}

	// A pool is shared with other users, so its totals cover their
	// sandboxes too. `instances` is always scoped to the current user, even for
	// admins, so summing its usage gives the caller's own share and the
	// remainder is everyone else's.
	function myUsageForPool(poolID: string) {
		return usageForAgent(
			poolID,
			instances.filter((instance) => (instance.poolID ?? '') === poolID)
		);
	}

	const formatCPU = (value: number) => `${Math.round(value * 100) / 100} vCPU`;
	const formatMemory = (value: number) => `${(value / 1024 ** 3).toFixed(2)} GiB`;

	function splitMetric(
		label: string,
		total: number,
		mine: number,
		capacity: number,
		format: (value: number) => string
	) {
		const others = Math.max(0, total - mine);
		const minePercent = percent(mine, capacity);
		return {
			label,
			minePercent,
			// Clamped against the share already drawn so rounding cannot push the
			// stacked bar past the track.
			othersPercent: Math.min(percent(others, capacity), 100 - minePercent),
			totalPercent: percent(total, capacity),
			// A native title renders newlines, so the figures go one per line
			// rather than in a single run of text nobody reads to the end of.
			//
			// Sharing a pool is the exception, and there is no way to know whether
			// this one is shared -- a user can only see their own assignment. So the
			// split is mentioned only when someone else is demonstrably using the
			// pool right now; otherwise the usage is simply the caller's own.
			title: (others > 0
				? [label, `Yours: ${format(mine)}`, `Others: ${format(others)}`]
				: [label, `Used: ${format(mine)}`]
			)
				.concat(`Capacity: ${format(capacity)}`)
				.join('\n')
		};
	}

	// Shown when the backend reports that it could not attribute disk usage to
	// this pool, which is why the bar reads "—" rather than 0%.
	const unmeasuredDiskTitle = [
		'Disk',
		'Usage cannot be measured on this cluster.',
		'',
		'The pool volume shares a filesystem with the node,',
		'so any figure would describe the host rather than',
		'this pool.'
	].join('\n');

	// Disk is not split between "yours" and "others" the way CPU and memory are.
	// Sandboxes share one volume separated by subPath, which the kubelet does not
	// measure per directory, so the only honest figure is the pool's total.
	function poolDiskMetric(poolID: string, capacity: number) {
		const snapshot = utilization.get(poolID);
		if (!snapshot?.storageMeasured) return undefined;
		const used = snapshot.pool.storageBytes;
		return {
			label: 'Disk',
			usedPercent: percent(used, capacity),
			title: [
				'Disk',
				`Used: ${formatMemory(used)}`,
				`Capacity: ${formatMemory(capacity)}`,
				'',
				'Counts every sandbox in the pool.'
			].join('\n')
		};
	}

	function usageForAgent(poolID: string, agentInstances: HostedAgentInstance[]) {
		const total: HostedAgentResourceQuantity = {
			cpuVcpus: 0,
			memoryBytes: 0,
			storageBytes: 0
		};
		for (const instance of agentInstances) {
			const usage = usageForInstance(poolID, instance.id);
			if (!usage) continue;
			total.cpuVcpus += usage.cpuVcpus;
			total.memoryBytes += usage.memoryBytes;
			total.storageBytes += usage.storageBytes;
		}
		return total;
	}

	async function refresh() {
		try {
			instances = await AdminService.listHostedAgentInstances();
			pools = await AdminService.listHostedAgentPools();
			await Promise.all(
				pools.map(async (pool) => {
					try {
						utilization.set(pool.id, await AdminService.getHostedAgentPoolUtilization(pool.id));
					} catch {
						utilization.delete(pool.id);
					}
				})
			);
		} catch {
			// Background observation is best-effort. A later poll or visibility
			// change retries without producing recurring notifications.
		}
	}

	function hasTransitionalInstances() {
		return instances.some(
			(instance) =>
				Boolean(instance.deleted) ||
				!instance.status?.state ||
				instance.status.state === 'pending' ||
				instance.status.state === 'removing'
		);
	}

	function instanceState(instance: HostedAgentInstance) {
		return instance.deleted ? 'removing' : (instance.status?.state ?? 'pending');
	}

	function scheduleRefresh(
		delay = hasTransitionalInstances() ? fastPollInterval : slowPollInterval
	) {
		clearTimeout(pollTimer);
		if (destroyed) return;
		pollTimer = setTimeout(poll, delay);
	}

	async function poll() {
		if (document.visibilityState === 'hidden') {
			scheduleRefresh(slowPollInterval);
			return;
		}
		await refresh();
		scheduleRefresh();
	}

	function create(agent: HostedAgent, poolID: string) {
		if (isAtInstanceLimit(agent)) return;
		formError = '';
		selectedAgent = agent;
		selectedPoolID = poolID;
		editing = undefined;
		form = { ...emptyForm(), answers: defaultAnswers(agent) };
		formDialog?.open();
	}

	function edit(agent: HostedAgent, instance: HostedAgentInstance) {
		formError = '';
		selectedAgent = agent;
		selectedPoolID = instance.poolID ?? '';
		editing = instance;
		form = {
			name: instance.name,
			description: instance.description ?? '',
			icon: instance.icon ?? '',
			answers: { ...defaultAnswers(agent), ...(instance.answers ?? {}) },
			gitRepo: instance.gitRepo ?? '',
			gitRef: instance.gitRef ?? '',
			mcpServers: [...(instance.mcpServers ?? [])],
			skills: [...(instance.skills ?? [])],
			models: [...(instance.models ?? [])]
		};
		formDialog?.open();
	}

	function handleFormError(error: unknown) {
		const parsed = parseErrorContent(error);
		formError =
			parsed.status === 400
				? parsed.message
				: error instanceof Error
					? error.message
					: String(error);
	}

	async function save() {
		if (!selectedAgent) return;
		formError = '';
		if (!editing && isAtInstanceLimit(selectedAgent)) {
			formError = `You have reached the limit of ${selectedAgent.maxInstancesPerUser} instances for ${selectedAgent.name}.`;
			return;
		}
		saving = true;
		try {
			const manifest = { ...form };
			if (editing) {
				await AdminService.updateHostedAgentInstance(editing.id, manifest, {
					errorHandler: handleFormError
				});
			} else {
				await AdminService.createHostedAgentInstance(
					{
						hostedAgentID: selectedAgent.id,
						poolID: selectedPoolID || undefined,
						...manifest
					},
					{ errorHandler: handleFormError }
				);
				expanded.add(`${selectedPoolID}:${selectedAgent.id}`);
			}
			await refresh();
			scheduleRefresh();
			formDialog?.close();
		} catch (error) {
			// The request-level handler normally sets this. Keep a fallback for
			// failures raised before a response reaches the HTTP layer.
			if (!formError) handleFormError(error);
		} finally {
			saving = false;
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

<Layout title="Hosted Agents">
	<div
		class="flex h-full w-full flex-col gap-5"
		in:fly={{ x: 100, duration }}
		out:fly={{ x: -100, duration }}
	>
		{#if hostedAgents.length === 0}
			<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
				<Bot class="text-muted-content size-24 opacity-25" />
				<h4 class="text-muted-content text-lg font-semibold">No hosted agents available</h4>
				<p class="text-muted-content text-sm font-light">
					Ask an administrator to grant you access to one.
				</p>
			</div>
		{:else}
			<!-- "Pool" is jargon on its own. Sharing one is the exception, so it is
			     left out here and surfaces in the bar's tooltip on the rare occasion
			     someone else is actually using the pool. -->
			<p class="text-muted-content -mt-1 text-sm font-light">
				Launch an agent to get your own instance of it. Instances run in a pool — a bucket of CPU
				and memory they draw from.
			</p>
			{#each displayedPools as pool, poolIndex (pool.id || 'pending')}
				{@const used = utilization.get(pool.id)?.pool}
				<section class="overflow-hidden rounded-md shadow-sm">
					<div
						class="bg-base-100 dark:bg-base-300 flex items-center justify-between gap-4 px-4 py-3"
					>
						<div class="min-w-0">
							<h2 class="text-sm font-semibold">
								{pool.id ? `Pool ${poolIndex + 1}` : 'Your pool'}
							</h2>
							<p class="text-muted-content truncate text-xs">
								{#if pool.id}
									{pool.id}
								{:else}
									Created automatically when you start your first instance
								{/if}
							</p>
						</div>
						<div class="flex items-center gap-4">
							{#if used}
								{@const mine = myUsageForPool(pool.id)}
								{@const disk = poolDiskMetric(pool.id, pool.capacity.storageBytes)}
								<!-- The pool is shared, so each bar is stacked: the solid share is the
								     caller's own sandboxes and the faded share is everyone else's. That
								     is what makes a busy pool alongside zero of your own instances read
								     as a fact rather than a bug. -->
								<div class="hidden w-64 gap-0.5 sm:grid">
									{#each [splitMetric('CPU', used.cpuVcpus, mine.cpuVcpus, pool.capacity.cpuVcpus, formatCPU), splitMetric('RAM', used.memoryBytes, mine.memoryBytes, pool.capacity.memoryBytes, formatMemory)] as metric (metric.label)}
										<div
											class="grid grid-cols-[2.25rem_1fr_2rem] items-center gap-1 text-[10px]"
											title={metric.title}
										>
											<span class="text-muted-content">{metric.label}</span>
											<div class="bg-base-200 dark:bg-base-400 flex h-1.5 overflow-hidden rounded">
												<div class="bg-primary h-full" style:width={`${metric.minePercent}%`}></div>
												<div
													class="bg-primary/30 h-full"
													style:width={`${metric.othersPercent}%`}
												></div>
											</div>
											<span class="text-muted-content text-right">{metric.totalPercent}%</span>
										</div>
									{/each}
									<!-- Disk gets a single bar, not a stacked one: the volume is shared
									     and the kubelet cannot attribute it per sandbox, so there is no
									     honest "yours" to draw. -->
									<div
										class="grid grid-cols-[2.25rem_1fr_2rem] items-center gap-1 text-[10px]"
										class:cursor-help={!disk}
										title={disk?.title ?? unmeasuredDiskTitle}
									>
										<span class="text-muted-content">Disk</span>
										{#if disk}
											<div class="bg-base-200 dark:bg-base-400 flex h-1.5 overflow-hidden rounded">
												<div class="bg-primary h-full" style:width={`${disk.usedPercent}%`}></div>
											</div>
											<span class="text-muted-content text-right">{disk.usedPercent}%</span>
										{:else}
											<div class="bg-base-200 dark:bg-base-400 h-1.5 rounded opacity-50"></div>
											<span class="text-muted-content text-right">—</span>
										{/if}
									</div>
								</div>
							{/if}
							{#if pool.suspended}<span class="badge badge-warning badge-sm">Suspended</span>{/if}
						</div>
					</div>
					<div
						class="text-muted-content bg-base-300 dark:bg-base-200 border-base-200 dark:border-base-400 grid grid-cols-[minmax(13rem,2fr)_6rem_minmax(8rem,1fr)_11rem] gap-3 border-t px-4 py-2 text-xs font-medium uppercase"
					>
						<span>Agent</span><span>Instances</span><span>Status</span><span></span>
					</div>
					{#each hostedAgents as agent (agent.id)}
						{@const rowInstances = instancesFor(agent.id, pool.id)}
						{@const agentUsage = usageForAgent(pool.id, rowInstances)}
						{@const rowKey = `${pool.id}:${agent.id}`}
						<div class="border-base-200 dark:border-base-400 border-t shadow-xs first:border-t-0">
							<div
								class="bg-base-100 hover:bg-base-200 dark:bg-base-300 dark:hover:bg-base-400 grid grid-cols-[minmax(13rem,2fr)_6rem_minmax(8rem,1fr)_11rem] items-center gap-3 px-4 py-2.5"
							>
								<button
									class="flex min-w-0 items-center gap-2 text-left"
									onclick={() =>
										expanded.has(rowKey) ? expanded.delete(rowKey) : expanded.add(rowKey)}
								>
									{#if expanded.has(rowKey)}<ChevronDown
											class="size-4 shrink-0"
										/>{:else}<ChevronRight class="size-4 shrink-0" />{/if}
									<AgentIcon icon={agent.icon} iconDark={agent.iconDark} alt="" />
									<div class="min-w-0">
										<p class="truncate text-sm font-medium">{agent.name}</p>
										{#if unavailableReason(agent)}
											<p class="text-warning truncate text-xs" title={unavailableReason(agent)}>
												Unavailable — {unavailableReason(agent)}
											</p>
										{:else}
											<p class="text-muted-content truncate text-xs">{agent.description ?? ''}</p>
										{/if}
									</div>
								</button>
								<span class="text-sm">{rowInstances.length}</span>
								<div class="text-muted-content min-w-0 text-xs">
									<p>
										{rowInstances.filter((item) => instanceState(item) === 'ready')
											.length}/{rowInstances.length} ready
									</p>
									{#if rowInstances.length}<p class="truncate">
											{agentUsage.cpuVcpus.toFixed(1)} CPU · {(
												agentUsage.memoryBytes / gibibyte
											).toFixed(1)}G RAM
										</p>{/if}
								</div>
								<button
									class="btn btn-ghost btn-sm"
									disabled={pool.suspended ||
										isAtInstanceLimit(agent) ||
										Boolean(unavailableReason(agent))}
									title={unavailableReason(agent)
										? `Cannot be launched here: ${unavailableReason(agent)}`
										: isAtInstanceLimit(agent)
											? `Limit of ${agent.maxInstancesPerUser} instances reached`
											: pool.suspended
												? 'This pool is suspended'
												: 'Create instance'}
									onclick={() => create(agent, pool.id)}><Plus class="size-4" /> New</button
								>
							</div>
							{#if expanded.has(rowKey)}
								{#each rowInstances as instance (instance.id)}
									{@const instanceUsage = usageForInstance(pool.id, instance.id)}
									<div
										class="bg-base-200 dark:bg-base-400 border-base-200 dark:border-base-400 grid grid-cols-[minmax(13rem,2fr)_6rem_minmax(8rem,1fr)_11rem] items-center gap-3 border-t px-4 py-2"
									>
										<div class="flex min-w-0 items-center gap-2 pl-6">
											<AgentIcon
												icon={instance.resolvedIcon}
												iconDark={instance.resolvedIconDark}
												alt=""
												class="size-4"
											/>
											<div class="min-w-0">
												<p class="truncate text-sm">{instance.name}</p>
												<p class="text-muted-content truncate text-xs">
													{instance.description ?? ''}
												</p>
											</div>
										</div>
										<span class="text-muted-content text-xs">Instance</span>
										<div>
											<span
												class="badge badge-sm {instanceState(instance) === 'ready'
													? 'badge-success'
													: instanceState(instance) === 'error'
														? 'badge-error'
														: 'badge-secondary'}">{instanceState(instance)}</span
											>
											<p class="text-muted-content mt-0.5 truncate text-xs">
												{instance.status?.message ??
													instance.status?.reason ??
													instance.status?.error ??
													''}
											</p>
											{#if instanceUsage}<p class="text-muted-content truncate text-[10px]">
													{instanceUsage.cpuVcpus.toFixed(1)} CPU · {(
														instanceUsage.memoryBytes / gibibyte
													).toFixed(1)}G RAM
												</p>{/if}
										</div>
										<div class="flex justify-end gap-1">
											<!-- status.url is the sandbox's in-cluster address, which a
											     browser cannot reach. Its presence means the agent declares a
											     port, so it doubles as the signal for whether there is
											     anything to open; the link itself goes through Obot. -->
											{#if instance.status?.url}<a
													class="btn btn-ghost btn-sm"
													href="/agent-connect/{instance.id}"
													target="_blank"
													rel="external noopener noreferrer"><ExternalLink class="size-4" /> Open</a
												>{/if}
											<!-- The terminal is the agent's decision, and a console only
											     exists while the sandbox is running. -->
											{#if agent.terminal}<a
													class="btn btn-ghost btn-sm {instanceState(instance) === 'ready'
														? ''
														: 'btn-disabled pointer-events-none opacity-50'}"
													aria-label="Open terminal"
													title={instanceState(instance) === 'ready'
														? 'Open terminal'
														: 'The sandbox is not running'}
													href={resolve('/hosted-agents/[instance_id]/terminal', {
														instance_id: instance.id
													})}><SquareTerminal class="size-4" /></a
												>{/if}
											<button
												class="btn btn-ghost btn-sm"
												aria-label="Edit instance"
												disabled={Boolean(instance.deleted)}
												onclick={() => edit(agent, instance)}><Pencil class="size-4" /></button
											>
											<button
												class="btn btn-ghost btn-sm text-error"
												aria-label="Delete instance"
												disabled={Boolean(instance.deleted)}
												onclick={() => (deleting = instance)}><Trash2 class="size-4" /></button
											>
										</div>
									</div>
								{:else}<p
										class="bg-base-200 dark:bg-base-400 border-base-200 dark:border-base-400 text-muted-content border-t px-10 py-3 text-xs"
									>
										No instances.
									</p>{/each}
							{/if}
						</div>
					{/each}
				</section>
			{/each}
		{/if}
	</div>
</Layout>

<ResponsiveDialog
	bind:this={formDialog}
	title={editing ? 'Edit instance' : `New ${selectedAgent?.name ?? 'instance'}`}
	class="default-scrollbar-thin max-h-[90vh] overflow-y-auto md:max-w-5xl"
>
	{#if selectedAgent}<HostedAgentInstanceForm
			agent={selectedAgent}
			bind:name={form.name}
			bind:description={form.description}
			bind:icon={form.icon}
			bind:answers={form.answers}
			bind:gitRepo={form.gitRepo}
			bind:gitRef={form.gitRef}
			bind:mcpServers={form.mcpServers}
			bind:skills={form.skills}
			bind:models={form.models}
		/>{/if}
	<div class="bg-base-100 dark:bg-base-300 sticky bottom-0 z-10 mt-4 border-t pt-3">
		{#if formError}
			<div class="notification-error mb-3 text-sm" role="alert">
				{formError}
			</div>
		{/if}
		<div class="flex justify-end gap-2">
			<button class="btn btn-secondary" onclick={() => formDialog?.close()}>Cancel</button><button
				class="btn btn-primary"
				disabled={saving ||
					!form.name ||
					(!editing && Boolean(selectedAgent && isAtInstanceLimit(selectedAgent)))}
				onclick={save}
				>{#if saving}<Loading class="size-4" />{:else}{editing ? 'Update' : 'Create'}{/if}</button
			>
		</div>
	</div>
</ResponsiveDialog>

<Confirm
	show={Boolean(deleting)}
	msg={`Delete ${deleting?.name ?? 'this instance'}?`}
	oncancel={() => (deleting = undefined)}
	onsuccess={async () => {
		if (!deleting) return;
		await AdminService.deleteHostedAgentInstance(deleting.id);
		deleting = undefined;
		await refresh();
		scheduleRefresh();
	}}
/>

<svelte:head><title>Obot | Hosted Agents</title></svelte:head>
