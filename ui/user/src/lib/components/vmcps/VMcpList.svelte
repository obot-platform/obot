<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { toInlineHTMLFromMarkdown } from '$lib/markdown';
	import type { OrgUser, VMCP } from '$lib/services';
	import type { VMcpComponentView, VMcpConnectOptions } from '$lib/services/vmcps/types';
	import { vmcpConnectURL } from '$lib/services/vmcps/utils';
	import { profile } from '$lib/stores';
	import { getUserDisplayName } from '$lib/utils';
	import McpServerIcon from './McpServerIcon.svelte';
	import VMcpCard from './VMcpCard.svelte';
	import VMcpIcon from './VMcpIcon.svelte';
	import type { Snippet } from 'svelte';

	const CARD_STAGGER_MS = 30;
	const CARD_STAGGER_MAX_STEPS = 8;

	interface Props {
		items: VMCP[];
		components: (vmcp: VMCP) => VMcpComponentView[];
		onSelect?: (vmcp: VMCP) => void;
		onConnect?: (vmcp: VMCP, options?: VMcpConnectOptions) => void;
		onDelete?: (vmcp: VMCP) => void;
		noDataContent?: Snippet;
		usersMap: Map<string, OrgUser>;
	}

	let { items, components, onSelect, onConnect, onDelete, noDataContent, usersMap }: Props =
		$props();

	let cards = $derived(items.map(toCard));
	let overflowHiddenById = $state<Record<string, number>>({});

	type VMcpListCard = ReturnType<typeof toCard>;

	function toCard(item: VMCP) {
		const componentServers = components(item);
		return {
			id: item.id,
			name: item.displayName || 'Untitled vMCP',
			connected: false,
			componentServers,
			descriptionHTML: toInlineHTMLFromMarkdown(item.description ?? ''),
			data: item
		};
	}

	function cardDelay(index: number) {
		return Math.min(index, CARD_STAGGER_MAX_STEPS) * CARD_STAGGER_MS;
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

	function getNote(vmcp: VMCP) {
		if (vmcp.userID) {
			return `Created by ${vmcp.userID === profile.current.id ? 'me' : getUserDisplayName(usersMap, vmcp.userID)}`;
		}
		return 'Created by admin';
	}
</script>

<div class="@container">
	{#if cards.length === 0}
		<div class="flex h-full items-center justify-center">
			{#if noDataContent}
				{@render noDataContent()}
			{:else}
				<p class="text-muted-content text-sm font-light">No vMCPs available.</p>
			{/if}
		</div>
	{:else}
		<div class="grid grid-cols-1 items-start gap-4 @2xl:grid-cols-2 @5xl:grid-cols-3">
			{#each cards as card, index (card.id)}
				{@render vmcpCard(card, index)}
			{/each}
		</div>
	{/if}
</div>

{#snippet vmcpCard(card: VMcpListCard, index: number)}
	{@const isOwner = card.data.userID === profile.current.id}
	<VMcpCard
		id={card.id}
		name={card.name}
		descriptionHTML={card.data.description ? card.descriptionHTML : undefined}
		connectURL={vmcpConnectURL(card.data)}
		connectButtonId={`btn-connect-to-server-${card.id}`}
		connected={card.connected}
		selectAriaLabel={`Open ${card.name}`}
		enterDelay={cardDelay(index)}
		onSelect={() => onSelect?.(card.data)}
		onConnect={(options) => onConnect?.(card.data, options)}
		onDelete={() => onDelete?.(card.data)}
		class="h-full text-base-content border-base-300 dark:border-base-400 bg-base-100 dark:bg-base-300 group @container cursor-pointer gap-3 rounded-lg border p-3 shadow-xs transition-[transform,box-shadow,border-color] duration-150 hover:border-primary hover:shadow-md"
		{isOwner}
		note={getNote(card.data)}
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

{#snippet serversPanel(card: VMcpListCard)}
	{@const hiddenCount = overflowHiddenById[card.id] ?? 0}
	{@const overflowed = hiddenCount > 0 ? card.componentServers.slice(-hiddenCount) : []}
	{#if card.componentServers.length === 0}
		<p class="text-muted-content py-2 text-center text-xs italic">
			No servers yet. Open this vMCP in the designer to add some.
		</p>
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
