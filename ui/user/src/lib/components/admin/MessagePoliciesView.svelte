<script lang="ts">
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Search from '$lib/components/Search.svelte';
	import Select from '$lib/components/Select.svelte';
	import MessagePolicyForm from '$lib/components/admin/MessagePolicyForm.svelte';
	import MessagePolicyViolationsView from '$lib/components/admin/MessagePolicyViolationsView.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { parseErrorContent } from '$lib/errors';
	import Loading from '$lib/icons/Loading.svelte';
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
		policyDirection?: Extract<PolicyDirection, 'user-message' | 'tool-calls'>;
		creating?: boolean;
	}

	let { messagePolicies: initialPolicies, policyDirection, creating = false }: Props = $props();

	let messagePolicies = $state<MessagePolicy[]>(untrack(() => initialPolicies));
	let query = $derived(page.url.searchParams.get('query') || '');

	$effect(() => {
		messagePolicies = initialPolicies;
	});

	type SinglePolicyDirection = Extract<PolicyDirection, 'user-message' | 'tool-calls'>;

	function isPolicyWithBothDirection(policy: MessagePolicy) {
		return policy.direction === 'both';
	}

	function chosenDirection(): SinglePolicyDirection | undefined {
		if (selectedDirection === 'user-message' || selectedDirection === 'tool-calls') {
			return selectedDirection;
		}
	}

	let policyToDelete = $state<MessagePolicy>();
	let policyToConvert = $state<MessagePolicy>();
	let convertCtrlClick = $state(false);
	let converting = $state<SinglePolicyDirection | 'delete'>();
	let convertError = $state('');
	let convertDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let confirmConvertDelete = $state(false);
	let selectedDirection = $state(untrack(() => policyDirection ?? 'user-message'));
	let directionChoices = $derived(
		(policyDirection === 'tool-calls'
			? (['tool-calls', 'user-message'] as const)
			: (['user-message', 'tool-calls'] as const)
		).map((direction) => ({
			id: direction,
			label: PolicyDirectionLabels[direction]
		}))
	);
	let isReadonly = $derived(profile.current.isAdminReadonly?.());
	let visiblePolicies = $derived(
		policyDirection
			? messagePolicies.filter(
					(policy) =>
						(policy.direction === policyDirection || isPolicyWithBothDirection(policy)) &&
						policy.displayName.toLowerCase().includes(query.toLowerCase())
				)
			: messagePolicies.filter((policy) =>
					policy.displayName.toLowerCase().includes(query.toLowerCase())
				)
	);
	let contentType = $derived<'policies' | 'policy-violations'>(
		(page.url.searchParams.get('contents') as 'policies' | 'policy-violations') || 'policies'
	);

	function convertToTableData(policy: MessagePolicy) {
		return {
			...policy,
			directionLabel: PolicyDirectionLabels[policy.direction as PolicyDirection] ?? policy.direction
		};
	}

	let tableData = $derived(visiblePolicies.map((policy) => convertToTableData(policy)));
	const duration = PAGE_TRANSITION_DURATION;

	function detailUrl(id: string, direction?: SinglePolicyDirection) {
		const directionToUse = direction ?? policyDirection;
		switch (directionToUse) {
			case 'tool-calls':
				return `/mcp-servers/message-policies/${id}`;
			case 'user-message':
				return `/models/message-policies/${id}`;
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
		goto(detailUrl(policy.id), { replaceState: false });
	}

	function promptToResolveBothDirection(policy: MessagePolicy, isCtrlClick: boolean) {
		policyToConvert = policy;
		convertCtrlClick = isCtrlClick;
		convertError = '';
		converting = undefined;
		selectedDirection = policyDirection ?? 'user-message';
		convertDialog?.open();
	}

	function handleConvertClose() {
		if (confirmConvertDelete) return;
		policyToConvert = undefined;
		convertError = '';
	}

	async function convertPolicy(direction: SinglePolicyDirection) {
		const policy = policyToConvert;
		if (!policy || converting) return;

		converting = direction;
		convertError = '';
		try {
			const updated = await AdminService.updateMessagePolicy(policy.id, {
				displayName: policy.displayName,
				definition: policy.definition,
				direction,
				subjects: policy.subjects
			});
			messagePolicies = messagePolicies.map((item) => (item.id === updated.id ? updated : item));
			const isCtrlClick = convertCtrlClick;
			policyToConvert = undefined;
			convertDialog?.close();
			openUrl(detailUrl(updated.id, direction), isCtrlClick);
		} catch (error) {
			convertError = parseErrorContent(error).message || 'Failed to update the policy direction.';
		} finally {
			converting = undefined;
		}
	}

	async function deleteConvertedPolicy() {
		const policy = policyToConvert;
		if (!policy || converting) return;

		converting = 'delete';
		convertError = '';
		try {
			await AdminService.deleteMessagePolicy(policy.id);
			messagePolicies = messagePolicies.filter((item) => item.id !== policy.id);
			confirmConvertDelete = false;
			policyToConvert = undefined;
			convertDialog?.close();
		} catch (error) {
			confirmConvertDelete = false;
			convertError = parseErrorContent(error).message || 'Failed to delete the policy.';
			convertDialog?.open();
		} finally {
			converting = undefined;
		}
	}
</script>

{#if creating}
	<div class="h-full w-full" in:fly={{ x: 100, delay: duration, duration }}>
		<MessagePolicyForm
			fixedDirection={policyDirection}
			onCreate={navigateToCreated}
			onCancel={closeCreate}
		/>
	</div>
{:else}
	<div class="flex flex-col gap-8" in:fade={{ duration }}>
		{#if messagePolicies.length === 0}
			<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
				<ShieldAlert class="text-base-content/80 size-24 opacity-25" />
				<h4 class="text-muted-content text-lg font-semibold">No message policies</h4>
				<p class="text-muted-content text-sm font-light">
					Looks like you don't have any message policies created yet. <br />
					{#if !isReadonly}
						Click the button below to get started.
					{/if}
				</p>

				{@render addPolicyButton()}
			</div>
		{:else}
			<div class="flex flex-col gap-2">
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
						Policies
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
						Policy Violations
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
							placeholder="Search message policies..."
						/>
					</div>
					{@render messagePolicyTable()}
				{:else if contentType === 'policy-violations'}
					<MessagePolicyViolationsView {policyDirection} />
				{/if}
			</div>
		{/if}
	</div>
{/if}

{#snippet messagePolicyTable()}
	<Table
		data={tableData}
		fields={['displayName']}
		onClickRow={(d, isCtrlClick) => {
			if (isPolicyWithBothDirection(d) && !isReadonly) {
				promptToResolveBothDirection(d, isCtrlClick);
			} else {
				openUrl(detailUrl(d.id), isCtrlClick);
			}
		}}
		setRowClasses={(d) => {
			if (isPolicyWithBothDirection(d)) {
				return 'bg-warning/10';
			}
			return '';
		}}
		headers={[
			{
				title: 'Name',
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
					tooltip={{ text: 'Delete' }}
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
			<Plus class="size-4" /> Add Message Policy
		</button>
	{/if}
{/snippet}

<Confirm
	msg={`Delete ${policyToDelete?.displayName || 'this policy'}?`}
	show={Boolean(policyToDelete)}
	onsuccess={async () => {
		if (!policyToDelete) return;
		await AdminService.deleteMessagePolicy(policyToDelete.id);
		messagePolicies = await AdminService.listMessagePolicies();
		policyToDelete = undefined;
	}}
	oncancel={() => (policyToDelete = undefined)}
/>

<ResponsiveDialog
	bind:this={convertDialog}
	title="Update Policy Direction"
	class="md:max-w-md"
	onClose={handleConvertClose}
>
	<div class="flex flex-col items-center gap-4 md:p-0 p-4">
		<p class="text-center text-base font-medium">
			{policyToConvert?.displayName || 'This policy'} needs to be updated.
		</p>
		<p class="text-sm font-light">
			Message policies now use one direction. Choose where this policy should apply.
		</p>
		{#if convertError}
			<p class="notification-error w-full p-3 text-sm">{convertError}</p>
		{/if}

		<div class="flex w-full flex-col gap-1 mb-4">
			<span id="message-policy-direction-label" class="text-sm font-light">Direction</span>
			<Select
				id="message-policy-direction"
				class="border-base-400 w-full border"
				classes={{ root: 'w-full' }}
				options={directionChoices}
				bind:selected={selectedDirection}
				ariaLabelledby="message-policy-direction-label"
				disabled={!!converting}
			/>
		</div>

		<div class="flex w-full flex-col gap-2">
			<button
				type="button"
				class="btn btn-primary w-full"
				disabled={!!converting || !chosenDirection()}
				onclick={() => {
					const direction = chosenDirection();
					if (direction) convertPolicy(direction);
				}}
			>
				{#if converting && converting !== 'delete'}
					<Loading class="size-4" />
				{:else}
					Save
				{/if}
			</button>
			<button
				type="button"
				class="btn btn-error w-full"
				disabled={!!converting}
				onclick={() => {
					if (!policyToConvert || converting) return;
					confirmConvertDelete = true;
					convertDialog?.close();
				}}
			>
				Delete
			</button>
		</div>
	</div>
</ResponsiveDialog>

<Confirm
	msg={`Delete ${policyToConvert?.displayName || 'this policy'}?`}
	show={confirmConvertDelete}
	loading={converting === 'delete'}
	onsuccess={deleteConvertedPolicy}
	oncancel={() => {
		if (converting === 'delete') return;
		confirmConvertDelete = false;
		policyToConvert = undefined;
		convertError = '';
	}}
/>
