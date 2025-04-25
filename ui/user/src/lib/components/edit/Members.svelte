<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { getLayout, openSidebarConfig } from '$lib/context/layout.svelte';
	import type { Project } from '$lib/services';
	import { Plus, X } from 'lucide-svelte';

	interface Props {
		project: Project;
	}

	let { project }: Props = $props();
	let layout = getLayout();
	let toDelete = $state('');
	const mockMembers = [
		{
			email: 'johndoe@gmail.com',
			iconURL:
				'https://fastly.picsum.photos/id/453/200/200.jpg?hmac=IO3u3eOcKSOUCe8J1IlvctdxPKLTh5wFXvBT4O3BNs4'
		},
		{
			email: 'janedoe@gmail.com',
			iconURL:
				'https://fastly.picsum.photos/id/348/200/200.jpg?hmac=3DFdqMmDkl3bpk6cV1tumcDAzASPQUSbXHXWZIbIvks'
		}
	];
</script>

<div class="flex flex-col gap-2">
	<div class="mb-1 flex items-center justify-between">
		<p class="grow text-sm font-semibold">Members</p>
		<button
			class="icon-button"
			onclick={() => openSidebarConfig(layout, 'members')}
			use:tooltip={'Add Member'}
		>
			<Plus class="size-5" />
		</button>
	</div>

	<div class="flex flex-col gap-2">
		{#each mockMembers as member}
			<div class="group flex w-full items-center rounded-md transition-colors duration-300">
				<button class="flex grow items-center gap-2 pl-1.5" onclick={() => {}}>
					<div class="size-6 overflow-hidden rounded-full bg-gray-50 dark:bg-gray-600">
						<img src={member.iconURL} class="h-full w-full object-cover" alt="agent member icon" />
					</div>
					<p class="w-[calc(100%-24px)] truncate text-left text-xs font-light">{member.email}</p>
				</button>
				<button
					class="py-2 pr-3 transition-opacity duration-200 group-hover:opacity-100 md:opacity-0"
					onclick={() => (toDelete = member.email)}
				>
					<X class="size-4" />
				</button>
			</div>
		{/each}
	</div>
</div>
