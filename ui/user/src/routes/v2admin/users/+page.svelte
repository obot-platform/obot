<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import Table from '$lib/components/Table.svelte';
	import { Role } from '$lib/services/admin/types';
	import { Trash2 } from 'lucide-svelte';
	import { fade } from 'svelte/transition';

	let { data } = $props();
	const { users } = data;

	const tableData = $derived(
		users.map((user) => ({
			...user,
			role: user.role === Role.ADMIN ? 'Admin' : 'User'
		}))
	);
</script>

<Layout>
	<div class="my-8" in:fade>
		<div class="flex flex-col gap-8">
			<div class="flex items-center justify-between">
				<h1 class="text-2xl font-semibold">Organization</h1>
			</div>

			<div class="flex flex-col gap-2">
				<h2 class="mb-2 text-lg font-semibold">Groups</h2>
				<Table data={[]} fields={[]}>
					{#snippet actions(d)}
						<button class="icon-button hover:text-red-500" onclick={() => {}}>
							<Trash2 class="size-4" />
						</button>
					{/snippet}
				</Table>
			</div>

			<div class="flex flex-col gap-2">
				<h2 class="mb-2 text-lg font-semibold">Users</h2>
				<Table data={tableData} fields={['email', 'role']}>
					{#snippet actions(d)}
						<button class="icon-button hover:text-red-500" onclick={() => {}}>
							<Trash2 class="size-4" />
						</button>
					{/snippet}
				</Table>
			</div>
		</div>
	</div>
</Layout>
