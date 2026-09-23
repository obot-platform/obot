<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { stripMarkdownToText } from '$lib/markdown';
	import { vmcpItemContext } from '$lib/runes/vmcps/vmcpItem.svelte';
	import { UserService, type OrgUser, type VMCP } from '$lib/services';
	import { MCP_CONNECTION_INVALID_LICENSE_MESSAGE } from '$lib/services/user/constants';
	import type { VMcpComponentView, VMcpConnectOptions } from '$lib/services/vmcps/types';
	import { getDisplayListText, getVMcpCreator } from '$lib/services/vmcps/utils';
	import { errors, profile, version, vmcpInstances } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import { goto } from '$lib/url';
	import McpServerIcon from './McpServerIcon.svelte';
	import VMcpActions from './VMcpActions.svelte';
	import VMcpCard from './VMcpCard.svelte';
	import VMcpIcon from './VMcpIcon.svelte';
	import VMcpMenuActions from './VMcpMenuActions.svelte';
	import VMcpStatusBadge from './VMcpStatusBadge.svelte';
	import { Ellipsis, MessageCircle, Plug, Trash2 } from '@lucide/svelte';
	import type { Snippet } from 'svelte';

	interface Props {
		items: VMCP[];
		components: (vmcp: VMCP) => VMcpComponentView[];
		onSelect?: (vmcp: VMCP) => void;
		onConnect?: (vmcp: VMCP, options?: VMcpConnectOptions) => void;
		onDelete?: (vmcp: VMCP) => void;
		onDeleted?: (vmcp: VMCP) => void;
		onUpdate?: (vmcp: VMCP) => void;
		noDataContent?: Snippet;
		usersMap: Map<string, OrgUser>;
		variant?: 'grid' | 'table';
	}

	let {
		items: initialItems,
		components,
		onSelect,
		onConnect,
		onDelete,
		onDeleted,
		onUpdate,
		noDataContent,
		usersMap,
		variant = 'grid'
	}: Props = $props();

	let items = $derived(initialItems.map(translateItem));
	let overflowHiddenById = $state<Record<string, number>>({});
	let vmcpActions = $state<ReturnType<typeof VMcpActions>>();
	let pendingBulkDelete = $state<VMCP[]>();
	let bulkDeleting = $state(false);

	let hasLicenseEntitlementViolations = $derived(
		(version.current.licenseEntitlementViolations || []).length > 0
	);

	type Item = ReturnType<typeof translateItem>;

	function translateItem(item: VMCP) {
		const componentServers = components(item);
		return {
			id: item.id,
			componentServers,
			vmcp: item,
			displayName: item.displayName || 'Untitled vMCP',
			description: item.description ?? '',
			owner: getVMcpCreator(item, usersMap, '') ?? '',
			serverNames: componentServers.map((component) => component.name),
			status: ''
		};
	}

	function canSelectRow(row: Item) {
		return vmcpItemContext(row.vmcp).canDelete;
	}

	function connectHandler(vmcp: VMCP, options?: VMcpConnectOptions) {
		if (onConnect) {
			onConnect(vmcp, options);
			return;
		}
		vmcpActions?.openConnect(vmcp, undefined, options);
	}

	function connectDisabled(canConnect: boolean) {
		return hasLicenseEntitlementViolations || !canConnect;
	}

	function connectDisabledMessage(canConnect: boolean) {
		if (hasLicenseEntitlementViolations) return MCP_CONNECTION_INVALID_LICENSE_MESSAGE;
		if (!canConnect) return 'Cannot connect or test a personal vMCP';
		return undefined;
	}

	function handleTest(vmcp: VMCP, toggle: (open?: boolean) => void) {
		const hasConfiguredInstance = vmcpInstances.current.items.some(
			(candidate) => candidate.vmcpID === vmcp.id && candidate.userID === profile.current.id
		);
		if (hasConfiguredInstance) {
			goto(`/vmcps/${vmcp.id}?view=inspector`);
			toggle(false);
			return;
		}
		connectHandler(vmcp, { onConnected: () => goto(`/vmcps/${vmcp.id}?view=inspector`) });
		toggle(false);
	}

	async function handleBulkDelete() {
		if (!pendingBulkDelete?.length) return;
		bulkDeleting = true;
		const deleted = pendingBulkDelete;
		try {
			for (const vmcp of deleted) {
				await UserService.deleteVMCP(vmcp.id);
				onDeleted?.(vmcp);
			}
			success.add(
				deleted.length === 1
					? `${deleted[0].displayName || 'vMCP'} deleted.`
					: `${deleted.length} vMCPs deleted.`
			);
		} catch {
			errors.append('Failed to delete vMCP(s).');
		} finally {
			bulkDeleting = false;
			pendingBulkDelete = undefined;
		}
	}

	function setOverflowHidden(cardId: string, hidden: number) {
		if (overflowHiddenById[cardId] === hidden) return;
		overflowHiddenById[cardId] = hidden;
	}

	function overflowRow(node: HTMLElement, params: { count: number; cardId: string }) {
		let current = params;

		function chips() {
			return [...node.querySelectorAll<HTMLElement>('[data-chip]')];
		}

		function moreEl() {
			return node.querySelector<HTMLElement>('[data-more]');
		}

		function measure() {
			const items = chips();
			const more = moreEl();
			if (items.length === 0) {
				setOverflowHidden(current.cardId, 0);
				return;
			}

			for (const item of items) {
				item.hidden = false;
			}
			if (more) more.hidden = true;

			const available = node.clientWidth;
			const gap = Number.parseFloat(getComputedStyle(node).columnGap) || 8;
			const widths = items.map((item) => item.offsetWidth);

			let used = 0;
			let visible = 0;
			for (let i = 0; i < items.length; i++) {
				const next = used + (i > 0 ? gap : 0) + widths[i];
				if (next <= available + 0.5) {
					used = next;
					visible = i + 1;
				} else {
					break;
				}
			}

			if (visible === items.length) {
				setOverflowHidden(current.cardId, 0);
				return;
			}

			if (more) {
				more.hidden = false;
				more.textContent = `+${items.length - Math.max(visible, 1)} more`;
				const moreWidth = more.offsetWidth + gap;
				while (visible > 0 && used + moreWidth > available + 0.5) {
					visible -= 1;
					used -= widths[visible] + (visible > 0 ? gap : 0);
				}
			}

			if (visible < 1) visible = 1;
			for (let i = 0; i < items.length; i++) {
				items[i].hidden = i >= visible;
			}
			if (more) {
				const hidden = items.length - visible;
				more.hidden = hidden <= 0;
				if (hidden > 0) {
					more.textContent = `+${hidden} more`;
				}
			}
			setOverflowHidden(current.cardId, Math.max(items.length - visible, 0));
		}

		const observer = new ResizeObserver(measure);
		observer.observe(node);
		requestAnimationFrame(measure);
		return {
			update(next: { count: number; cardId: string }) {
				current = next;
				requestAnimationFrame(measure);
			},
			destroy() {
				observer.disconnect();
			}
		};
	}
</script>

<div class="@container">
	{#if items.length === 0}
		<div class="flex h-full items-center justify-center">
			{#if noDataContent}
				{@render noDataContent()}
			{:else}
				<p class="text-muted-content text-sm font-light">No vMCPs available.</p>
			{/if}
		</div>
	{:else}
		{#if variant === 'grid'}
			<div class="grid grid-cols-1 items-start gap-4 @2xl:grid-cols-2 @5xl:grid-cols-3">
				{#each items as item (item.id)}
					{@render vmcpCard(item)}
				{/each}
			</div>
		{:else}
			{@render table()}
		{/if}
	{/if}
</div>

<VMcpActions bind:this={vmcpActions} />

<Confirm
	show={Boolean(pendingBulkDelete?.length)}
	onsuccess={handleBulkDelete}
	oncancel={() => (pendingBulkDelete = undefined)}
	msg=""
	loading={bulkDeleting}
	title="Confirm Delete"
>
	{#snippet note()}
		Are you sure you want to delete
		{#if pendingBulkDelete?.length === 1}
			"<b>{pendingBulkDelete[0].displayName ?? 'this vMCP'}</b>"?
		{:else}
			<b>{pendingBulkDelete?.length ?? 0} vMCPs</b>?
		{/if}
		This cannot be undone.
	{/snippet}
</Confirm>

{#snippet table()}
	<div class="dark:bg-base-300 bg-base-100 rounded-md shadow-sm">
		<Table
			data={items}
			fields={['displayName', 'description', 'owner', 'status', 'serverNames']}
			headers={[
				{ title: 'Name', property: 'displayName' },
				{ title: 'Servers', property: 'serverNames' },
				{ title: 'Created by', property: 'owner' }
			]}
			noDataMessage="No vMCPs available."
			noAutoHideFields={['displayName']}
			validateSelect={profile.current.isAdmin?.() ? undefined : canSelectRow}
			disabledSelectMessage="You can only delete vMCPs you created."
			remeasureKey={JSON.stringify(overflowHiddenById)}
			setRowClasses={() => 'group'}
			onClickRow={(row) => onSelect?.(row.vmcp)}
			classes={{
				root: 'rounded-none rounded-b-md shadow-none'
			}}
		>
			{#snippet onRenderColumn(property, row)}
				{#if property === 'displayName'}
					<div class="flex min-w-0 items-center gap-2">
						<VMcpIcon class="size-8" components={row.componentServers} />
						<p class="truncate" title={row.displayName}>{row.displayName}</p>
					</div>
				{:else if property === 'description'}
					<p class="line-clamp-1" title={row.description}>
						{stripMarkdownToText(row.description) || '—'}
					</p>
				{:else if property === 'serverNames'}
					{@const serverNames = getDisplayListText(row.serverNames, 3)}
					{#if serverNames}
						<p class="line-clamp-1" title={serverNames}>
							{serverNames}
						</p>
					{:else}
						<p class="line-clamp-1" title="—">—</p>
					{/if}
				{:else if property === 'status'}
					<VMcpStatusBadge
						vmcp={row.vmcp}
						showEmpty
						onUpdated={onUpdate}
						openSelectInstance={vmcpActions?.openSelectInstance}
						openUpdateConfirm={vmcpActions?.openUpdateConfirm}
						openEditInstanceConfiguration={vmcpActions?.openEditInstanceConfiguration}
					/>
				{:else}
					{row[property as keyof typeof row]}
				{/if}
			{/snippet}

			{#snippet actions(row)}
				{@const ctx = vmcpItemContext(row.vmcp)}
				{#if ctx.canConnect || ctx.hasActions}
					<DotDotDot
						class="hover:dark:bg-base-100/50"
						classes={{ menu: 'min-w-48' }}
						ariaLabel={`Actions for ${ctx.name}`}
					>
						{#snippet icon()}
							<Ellipsis class="size-4" />
						{/snippet}

						{#snippet children({ toggle })}
							{#if ctx.canConnect}
								<button
									class="menu-button"
									disabled={connectDisabled(ctx.canConnect)}
									use:tooltip={{ text: connectDisabledMessage(ctx.canConnect) }}
									onclick={(e) => {
										e.stopPropagation();
										connectHandler(row.vmcp);
										toggle(false);
									}}
								>
									<Plug class="size-4" /> Connect
								</button>
								<button
									class="menu-button"
									disabled={connectDisabled(ctx.canConnect)}
									use:tooltip={{ text: connectDisabledMessage(ctx.canConnect) }}
									onclick={(e) => {
										e.stopPropagation();
										handleTest(row.vmcp, toggle);
									}}
								>
									<MessageCircle class="size-4" /> Test vMCP
								</button>
							{/if}
							{#if ctx.hasActions}
								<VMcpMenuActions
									vmcp={row.vmcp}
									{toggle}
									onDelete={() => onDelete?.(row.vmcp)}
									onUpdated={onUpdate}
									openSelectInstance={vmcpActions?.openSelectInstance}
									openDiff={vmcpActions?.openDiff}
									openUpdateConfirm={vmcpActions?.openUpdateConfirm}
									openEditInstanceConfiguration={vmcpActions?.openEditInstanceConfiguration}
								/>
							{/if}
						{/snippet}
					</DotDotDot>
				{/if}
			{/snippet}

			{#snippet tableSelectActions(currentSelected)}
				{@const deletable = Object.values(currentSelected).filter(canSelectRow)}
				<div class="flex grow items-center justify-end gap-2 px-4 py-2">
					<button
						class="btn btn-secondary flex items-center gap-1 text-sm font-normal"
						onclick={() => (pendingBulkDelete = deletable.map((row) => row.vmcp))}
						disabled={deletable.length === 0}
					>
						<Trash2 class="size-4" /> Delete
						{#if deletable.length > 0}
							<span class="pill-primary">{deletable.length}</span>
						{/if}
					</button>
				</div>
			{/snippet}
		</Table>
	</div>
{/snippet}

{#snippet vmcpCard(card: Item)}
	<VMcpCard
		vmcp={card.vmcp}
		selectAriaLabel={`Open ${card.vmcp.displayName || 'Untitled vMCP'}`}
		onSelect={() => onSelect?.(card.vmcp)}
		onConnect={(options) =>
			onConnect
				? onConnect(card.vmcp, options)
				: vmcpActions?.openConnect(card.vmcp, undefined, options)}
		onDelete={() => onDelete?.(card.vmcp)}
		{onUpdate}
		openSelectInstance={vmcpActions?.openSelectInstance}
		openDiff={vmcpActions?.openDiff}
		openUpdateConfirm={vmcpActions?.openUpdateConfirm}
		openEditInstanceConfiguration={vmcpActions?.openEditInstanceConfiguration}
		class="h-full text-base-content border-base-300 dark:border-base-400 bg-base-100 dark:bg-base-300 group @container cursor-pointer gap-3 rounded-lg border p-3 shadow-xs transition-[transform,box-shadow,border-color] duration-150 hover:border-primary hover:shadow-md"
		owner={getVMcpCreator(card.vmcp, usersMap)}
	>
		{#snippet icon()}
			<VMcpIcon components={card.componentServers} />
		{/snippet}
		{@render serversPanel(card)}
	</VMcpCard>
{/snippet}

{#snippet serverChip(component: VMcpComponentView, asRowChip = false)}
	<div
		data-chip={asRowChip ? true : undefined}
		data-name={asRowChip ? component.name : undefined}
		class="bg-base-100 dark:bg-base-300 border-base-300 dark:border-base-400 group-hover:border-primary/40 flex shrink-0 items-center gap-2 rounded-md border pr-2 transition-colors"
	>
		<McpServerIcon
			icon={component.icon}
			width={12}
			height={12}
			class="size-3"
			classes={{ root: 'rounded-r-none' }}
		/>
		<span class="text-xs whitespace-nowrap">{component.name}</span>
	</div>
{/snippet}

{#snippet serversPanel(card: Item)}
	{@const hiddenCount = overflowHiddenById[card.id] ?? 0}
	{@const overflowed = hiddenCount > 0 ? card.componentServers.slice(-hiddenCount) : []}
	{#if card.componentServers.length === 0}
		{#if variant === 'table'}
			<span class="text-muted-content text-xs italic">No servers</span>
		{:else}
			<p class="text-muted-content py-2 text-center text-xs italic">
				No servers yet. Open this vMCP in the designer to add some.
			</p>
		{/if}
	{:else}
		<div
			class="flex flex-nowrap items-center gap-2 overflow-hidden"
			use:overflowRow={{ count: card.componentServers.length, cardId: card.id }}
		>
			{#each card.componentServers as component (component.key)}
				{@render serverChip(component, true)}
			{/each}
			{#snippet moreContent()}
				<div class="flex max-w-xs flex-wrap gap-1 text-left font-normal">
					{#each overflowed as component (component.key)}
						{@render serverChip(component)}
					{/each}
				</div>
			{/snippet}
			{#key hiddenCount}
				<div
					data-more
					hidden={hiddenCount <= 0}
					aria-label={hiddenCount > 0 ? `${hiddenCount} more servers` : undefined}
					class="pointer-events-auto relative z-10 border-base-400 text-muted-content flex shrink-0 items-center justify-center rounded-md border border-dashed px-1.5 py-1 font-mono text-xs whitespace-nowrap"
					use:tooltip={hiddenCount > 0
						? {
								snippet: moreContent,
								placement: 'top',
								classes: ['tooltip-surface', 'w-fit']
							}
						: undefined}
				></div>
			{/key}
		</div>
	{/if}
{/snippet}
