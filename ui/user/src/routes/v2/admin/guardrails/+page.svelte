<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/Table.svelte';
	import { ChevronLeft, Plus, TrainTrack, Trash2 } from 'lucide-svelte';
	import { fly } from 'svelte/transition';
	import { goto } from '$app/navigation';
	import Confirm from '$lib/components/Confirm.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { onMount } from 'svelte';
	import { AdminService } from '$lib/services/index.js';
	import GuardrailForm from '$lib/components/admin/GuardrailForm.svelte';

	type Guardrail = {
		id: string;
		displayName: string;
		urls: string[];
		models: {
			modelProviderID: string;
			id: string;
			name: string;
		}[];
	};

	let showCreateGuardrail = $state(false);
	let guardrailToDelete = $state<Guardrail>();
	let guardrails = $state<Guardrail[]>([]);

	onMount(() => {
		const url = new URL(window.location.href);
		const queryParams = new URLSearchParams(url.search);
		if (queryParams.get('new')) {
			showCreateGuardrail = true;
		}
	});

	function handleNavigation(url: string) {
		goto(url, { replaceState: false });
	}

	// async function navigateToCreated(filterId: string) {
	// 	showCreateFilter = false;
	// 	goto(`/v2/admin/filters/${filterId}`, { replaceState: false });
	// }

	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout>
	<div
		class="my-4 h-full w-full"
		in:fly={{ x: 100, duration, delay: duration }}
		out:fly={{ x: -100, duration }}
	>
		{#if showCreateGuardrail}
			{@render createGuardrailScreen()}
		{:else}
			<div
				class="flex flex-col gap-8"
				in:fly={{ x: 100, delay: duration, duration }}
				out:fly={{ x: -100, duration }}
			>
				<div class="flex items-center justify-between">
					<h1 class="text-2xl font-semibold">Guardrails</h1>
					{#if guardrails.length > 0}
						<div class="relative flex items-center gap-4">
							{@render addGuardrailButton()}
						</div>
					{/if}
				</div>
				{#if guardrails.length === 0}
					<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
						<TrainTrack class="size-24 text-gray-200 dark:text-gray-900" />
						<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">
							No created guardrails
						</h4>
						<p class="text-sm font-light text-gray-400 dark:text-gray-600">
							Looks like you don't have any guardrails created yet. <br />
							Click the button below to get started.
						</p>

						{@render addGuardrailButton()}
					</div>
				{:else}
					<Table
						data={guardrails}
						fields={['displayName', 'models']}
						onSelectRow={(d) => {
							handleNavigation(`/v2/admin/guardrails/${d.id}`);
						}}
						headers={[
							{
								title: 'Name',
								property: 'displayName'
							}
						]}
					>
						{#snippet actions(d)}
							<button
								class="icon-button hover:text-red-500"
								onclick={(e) => {
									e.stopPropagation();
									guardrailToDelete = d;
								}}
								use:tooltip={'Delete Guardrail'}
							>
								<Trash2 class="size-4" />
							</button>
						{/snippet}
						{#snippet onRenderColumn(property, d)}
							{#if property === 'models'}
								{@const count = d.models.length}
								{count ? count : '-'}
							{:else}
								{d[property as keyof typeof d]}
							{/if}
						{/snippet}
					</Table>
				{/if}
			</div>
		{/if}
	</div>
</Layout>

{#snippet addGuardrailButton()}
	<button
		class="button-primary flex items-center gap-1 text-sm"
		onclick={() => (showCreateGuardrail = true)}
	>
		<Plus class="size-4" /> Add New Guardrail
	</button>
{/snippet}

{#snippet createGuardrailScreen()}
	<div
		class="h-full w-full"
		in:fly={{ x: 100, delay: duration, duration }}
		out:fly={{ x: -100, duration }}
	>
		<GuardrailForm>
			{#snippet topContent()}
				<div class="flex flex-wrap items-center">
					<button
						onclick={() => (showCreateGuardrail = false)}
						class="button-text flex -translate-x-1 items-center gap-2 p-0 text-lg font-light"
					>
						<ChevronLeft class="size-4" />
						Guardrails
					</button>
					<ChevronLeft class="mx-2 size-4" />
					<span class="text-lg font-light">Create Guardrail</span>
				</div>
			{/snippet}
		</GuardrailForm>
	</div>
{/snippet}

<Confirm
	msg="Are you sure you want to delete this guardrail?"
	show={Boolean(guardrailToDelete)}
	onsuccess={async () => {
		if (!guardrailToDelete) return;
		await AdminService.deleteAccessControlRule(guardrailToDelete.id);
		// filters = await [fetchGuardrails]
		guardrailToDelete = undefined;
	}}
	oncancel={() => (guardrailToDelete = undefined)}
/>

<svelte:head>
	<title>Obot | Guardrails</title>
</svelte:head>
