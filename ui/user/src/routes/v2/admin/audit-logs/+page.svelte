<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { Captions } from 'lucide-svelte';

	import { fade } from 'svelte/transition';

	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout>
	<div class="my-4" in:fade={{ duration }} out:fade={{ duration }}>
		<div class="flex flex-col gap-8">
			<h1 class="text-2xl font-semibold">Audit Logs</h1>
			<div class="mt-12 flex w-md flex-col items-center gap-4 self-center text-center">
				<Captions class="size-24 text-gray-200 dark:text-gray-900" />
				<h4 class="text-lg font-semibold text-gray-400 dark:text-gray-600">No audit logs</h4>
				<p class="text-sm font-light text-gray-400 dark:text-gray-600">
					Currently, there are no audit logs.
				</p>
			</div>
		</div>
	</div>
</Layout>

<svelte:head>
	<title>Obot | Audit Logs</title>
</svelte:head>
