<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Search from '$lib/components/Search.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import { AdminService, type AccessControlRule } from '$lib/services';
	import { BookOpenText, Check } from '@lucide/svelte';
	import { debounce } from 'es-toolkit';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		onAdd: (policies: AccessControlRule[]) => void;
		filterIds?: string[];
	}

	let { onAdd, filterIds }: Props = $props();

	let addPolicyDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let policies = $state<AccessControlRule[]>([]);
	let loading = $state(false);
	let searchNames = $state('');
	let selectedPolicies = $state<AccessControlRule[]>([]);
	let selectedPoliciesMap = $derived(new Set(selectedPolicies.map((policy) => policy.id)));
	let filteredPolicies = $state<AccessControlRule[]>([]);

	let filteredData = $derived.by(() => {
		const filterIdSet = new Set(filterIds ?? []);
		return filteredPolicies.filter(
			(policy) =>
				!filterIdSet.has(policy.id) && !policy.resources?.some((resource) => resource.id === '*')
		);
	});

	async function search() {
		const query = searchNames.trim().toLowerCase();
		filteredPolicies = query
			? policies.filter((policy) => (policy.displayName ?? '').toLowerCase().includes(query))
			: policies;
	}

	const handleSearch = debounce(() => {
		search();
	}, 500);

	export function open() {
		searchNames = '';
		addPolicyDialog?.open();
	}

	async function onOpen() {
		loading = true;

		try {
			policies = await AdminService.listAccessControlRules();
		} catch (error) {
			console.error('Error loading access policies:', error);
			policies = [];
		} finally {
			loading = false;
		}

		await search();
	}

	function onClose() {
		loading = false;
		searchNames = '';
		selectedPolicies = [];
		filteredPolicies = [];
	}

	function subjectCountLabel(policy: AccessControlRule) {
		const count = policy.subjects?.length ?? 0;
		if (count === 0) return 'No subjects';
		return count === 1 ? '1 subject' : `${count} subjects`;
	}
</script>

<ResponsiveDialog
	id="add-access-policy-dialog"
	bind:this={addPolicyDialog}
	{onClose}
	{onOpen}
	title="Add Access Policy"
	class="h-full w-full overflow-visible md:h-125 md:max-w-md"
	classes={{ header: 'p-4 md:pb-0', content: 'min-h-inherit p-0' }}
>
	<div class="default-scrollbar-thin flex grow flex-col gap-4 overflow-y-auto pt-1">
		<div class="px-4">
			<Search
				class="dark:bg-base-200 dark:border-base-400 shadow-inner dark:border"
				value={searchNames}
				onChange={(val) => {
					searchNames = val;
					handleSearch();
				}}
				placeholder="Search by access policy name..."
			/>
		</div>
		{#if loading}
			<div class="flex grow items-center justify-center">
				<Loading class="size-6" />
			</div>
		{:else if filteredData.length === 0}
			<div
				class="text-muted-content flex grow items-center justify-center px-4 text-center text-sm"
			>
				No access policies found.
			</div>
		{:else}
			<div class="flex flex-col">
				{#each filteredData as policy (policy.id)}
					<button
						class={twMerge(
							'dark:hover:bg-base-200 hover:bg-base-400 flex items-center gap-2 px-4 py-2 text-left',
							selectedPoliciesMap.has(policy.id) && 'bg-base-200/50'
						)}
						onclick={() => {
							if (selectedPoliciesMap.has(policy.id)) {
								selectedPolicies = selectedPolicies.filter((item) => item.id !== policy.id);
							} else {
								selectedPolicies = [...selectedPolicies, policy];
							}
						}}
					>
						<div class="flex grow flex-col">
							<p>{policy.displayName}</p>
							<p class="text-muted-content font-light">{subjectCountLabel(policy)}</p>
						</div>
						<div class="flex items-center justify-center">
							{#if selectedPoliciesMap.has(policy.id)}
								<Check class="text-primary size-6" />
							{/if}
						</div>
					</button>
				{/each}
			</div>
		{/if}
	</div>
	<div class="flex w-full flex-col justify-between gap-4 p-4 md:flex-row">
		<div class="flex items-center gap-1 font-light">
			{#if selectedPolicies.length > 0}
				<BookOpenText class="size-4" />
				{selectedPolicies.length} Selected
			{/if}
		</div>
		<div class="flex items-center gap-2">
			<button class="btn btn-secondary w-full md:w-fit" onclick={() => addPolicyDialog?.close()}>
				Cancel
			</button>
			<button
				class="btn btn-primary w-full md:w-fit"
				onclick={() => {
					onAdd(selectedPolicies);
					addPolicyDialog?.close();
				}}
			>
				Confirm
			</button>
		</div>
	</div>
</ResponsiveDialog>
