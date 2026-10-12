<script lang="ts">
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import Search from '$lib/components/Search.svelte';
	import MessagePolicyForm from '$lib/components/admin/MessagePolicyForm.svelte';
	import MessagePolicyViolationsView from '$lib/components/admin/MessagePolicyViolationsView.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { m } from '$lib/i18n';
	import {
		type MessagePolicy,
		type PolicyDirection,
		PolicyDirectionLabels
	} from '$lib/services/admin/types';
	import { AdminService } from '$lib/services/index.js';
	import { profile } from '$lib/stores/index.js';
	import { goto, clearUrlParams } from '$lib/url';
	import { setUrlParamAndUpdateUrl } from '$lib/url';
	import { openUrl } from '$lib/utils.js';
	import { ShieldAlert, Plus, Trash2 } from '@lucide/svelte';
	import { untrack } from 'svelte';
	import { fade, fly } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		messagePolicies: MessagePolicy[];
		policyDirection: Extract<PolicyDirection, 'user-message' | 'tool-calls'>;
		creating?: boolean;
	}

	let { messagePolicies: initialPolicies, policyDirection, creating = false }: Props = $props();

	let messagePolicies = $state<MessagePolicy[]>(untrack(() => initialPolicies));
	let query = $derived(page.url.searchParams.get('query') || '');

	$effect(() => {
		messagePolicies = initialPolicies;
	});

	function isPolicyWithBothDirection(policy: MessagePolicy) {
		return policy.direction === 'both';
	}

	let policyToDelete = $state<MessagePolicy>();
	let isReadonly = $derived(profile.current.isAdminReadonly?.());
	let visiblePolicies = $derived(
		messagePolicies.filter(
			(policy) => policy.direction === policyDirection || isPolicyWithBothDirection(policy)
		)
	);
	let contentType = $derived<'policies' | 'policy-violations'>(
		page.url.searchParams.get('contents') === 'policy-violations' ? 'policy-violations' : 'policies'
	);

	function convertToTableData(policy: MessagePolicy) {
		return {
			...policy,
			directionLabel: PolicyDirectionLabels[policy.direction as PolicyDirection] ?? policy.direction
		};
	}

	let tableData = $derived(
		visiblePolicies
			.filter((policy) => policy.displayName.toLowerCase().includes(query.toLowerCase()))
			.map((policy) => convertToTableData(policy))
	);
	const duration = PAGE_TRANSITION_DURATION;

	function detailUrl(id: string, direction: PolicyDirection = policyDirection) {
		const resolved = direction === 'both' ? policyDirection : direction;
		switch (resolved) {
			case 'tool-calls':
				return `/mcp-servers/ai-judge-policies/${id}`;
			case 'user-message':
				return `/models/ai-judge-policies/${id}`;
			default:
				return '';
		}
	}

	function closeCreate() {
		const url = new URL(page.url);
		url.searchParams.delete('new');
		goto(url, { replaceState: true });
	}

	function openCreate() {
		const url = new URL(page.url);
		url.searchParams.set('new', 'true');
		goto(url);
	}

	async function navigateToCreated(policy: MessagePolicy) {
		clearUrlParams(['new']);
		goto(detailUrl(policy.id, policy.direction), { replaceState: false });
	}
</script>

{#if creating}
	<div class="h-full w-full" in:fly={{ x: 100, delay: duration, duration }}>
		<MessagePolicyForm
			fixedDirection={policyDirection}
			onCreate={navigateToCreated}
			onCancel={closeCreate}
			listHref={policyDirection === 'tool-calls'
				? '/mcp-servers?view=ai-judge-policies'
				: '/models?view=ai-judge-policies'}
		/>
	</div>
{:else}
	<div class="flex flex-col gap-2" in:fade={{ duration }}>
		<div class="tabs tabs-box bg-base-100 shadow-sm dark:bg-base-300 w-fit">
			<button
				class={twMerge(
					'tab text-xs min-w-24',
					contentType === 'policies' && 'tab-active bg-base-300 dark:bg-base-100'
				)}
				onclick={() => {
					setUrlParamAndUpdateUrl(page.url, 'contents', 'policies');
				}}
			>
				{m.ai_judge_policies()}
			</button>
			<button
				class={twMerge(
					'tab text-xs min-w-24',
					contentType === 'policy-violations' && 'tab-active bg-base-300 dark:bg-base-100'
				)}
				onclick={() => {
					setUrlParamAndUpdateUrl(page.url, 'contents', 'policy-violations');
				}}
			>
				{m.ai_judge_policy_violations()}
			</button>
		</div>
		{#if contentType === 'policies'}
			<div class="bg-base-200 dark:bg-base-100 sticky top-16 left-0 z-20 w-full py-1">
				<Search
					value={query}
					class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
					onChange={(value) => {
						setUrlParamAndUpdateUrl(page.url, 'query', value);
					}}
					placeholder={m.core_search_policies()}
				/>
			</div>
			{#if visiblePolicies.length === 0}
				<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
					<ShieldAlert class="text-base-content/80 size-24 opacity-25" />
					<h4 class="text-muted-content text-lg font-semibold">
						{m.core_no_ai_judge_policies()}
					</h4>
					<p class="text-muted-content text-sm font-light">
						{m.core_no_ai_judge_policies_yet()} <br />
						{#if !isReadonly}
							{m.core_click_below_to_start()}
						{/if}
					</p>

					{@render addPolicyButton()}
				</div>
			{:else}
				{@render messagePolicyTable()}
			{/if}
		{:else if contentType === 'policy-violations'}
			<MessagePolicyViolationsView {policyDirection} />
		{/if}
	</div>
{/if}

{#snippet messagePolicyTable()}
	<Table
		data={tableData}
		fields={['displayName']}
		onClickRow={(d, isCtrlClick) => {
			openUrl(detailUrl(d.id, d.direction), isCtrlClick);
		}}
		headers={[
			{
				title: m.core_name(),
				property: 'displayName'
			}
		]}
		filterable={['displayName']}
		sortable={['displayName']}
	>
		{#snippet actions(d)}
			{#if !isReadonly}
				<IconButton
					variant="danger"
					onclick={(e) => {
						e.stopPropagation();
						policyToDelete = d;
					}}
					tooltip={{ text: m.core_delete() }}
				>
					<Trash2 class="size-4" />
				</IconButton>
			{/if}
		{/snippet}
	</Table>
{/snippet}

{#snippet addPolicyButton()}
	{#if !isReadonly}
		<button class="btn btn-primary flex items-center gap-1 text-sm" onclick={openCreate}>
			<Plus class="size-4" />
			{m.core_add_ai_judge_policy()}
		</button>
	{/if}
{/snippet}

<Confirm
	msg={policyToDelete?.displayName
		? m.core_delete_named_component({ name: policyToDelete.displayName })
		: m.core_delete_this_policy()}
	show={Boolean(policyToDelete)}
	onsuccess={async () => {
		if (!policyToDelete) return;
		await AdminService.deleteMessagePolicy(policyToDelete.id);
		messagePolicies = await AdminService.listMessagePolicies();
		policyToDelete = undefined;
	}}
	oncancel={() => (policyToDelete = undefined)}
/>
