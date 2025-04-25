<script lang="ts">
	import { X } from 'lucide-svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import SearchDropdown from '$lib/components/SearchDropdown.svelte';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import { browser } from '$app/environment';
	import { ChatService, type Project, type ProjectShare } from '$lib/services';

	let toDelete = $state('');

	interface Props {
		project: Project;
	}

	let { project }: Props = $props();

	const mockMembers = [
		{
			id: '1',
			name: 'johndoe@gmail.com',
			email: 'johndoe@gmail.com',
			iconURL:
				'https://fastly.picsum.photos/id/453/200/200.jpg?hmac=IO3u3eOcKSOUCe8J1IlvctdxPKLTh5wFXvBT4O3BNs4'
		},
		{
			id: '2',
			name: 'janedoe@gmail.com',
			email: 'janedoe@gmail.com',
			iconURL:
				'https://fastly.picsum.photos/id/348/200/200.jpg?hmac=3DFdqMmDkl3bpk6cV1tumcDAzASPQUSbXHXWZIbIvks'
		}
	];

	let share = $state<ProjectShare>();
	let url = $derived(
		browser && share?.publicID
			? `${window.location.protocol}//${window.location.host}/s/${share.publicID}`
			: ''
	);

	async function updateShare() {
		share = await ChatService.getProjectShare(project.assistantID, project.id);
	}

	$effect(() => {
		if (project) {
			updateShare();
		}
	});

	async function handleChange(checked: boolean) {
		if (checked) {
			share = await ChatService.createProjectShare(project.assistantID, project.id);
		} else {
			await ChatService.deleteProjectShare(project.assistantID, project.id);
			share = undefined;
		}
	}
</script>

<CollapsePane classes={{ header: 'pl-3 py-2 text-md', content: 'p-0' }} iconSize={5}>
	{#snippet header()}
		<span class="flex grow items-center gap-2 text-start text-sm font-extralight"> Sharing </span>
	{/snippet}
	<div class="flex flex-col">
		<CollapsePane
			classes={{
				header: 'pl-3 pr-5.5 py-2 border-surface3 border-b',
				content: 'p-3 border-b border-surface3'
			}}
			iconSize={4}
		>
			{#snippet header()}
				<p class="w-full text-left text-sm font-normal">Members</p>
			{/snippet}
			<div class="flex flex-col gap-2 text-sm">
				<p class="py-2 text-xs font-light text-gray-500">
					Modify who has access to collaborate on your agent.
				</p>
				<SearchDropdown
					items={mockMembers}
					onSearch={() => {}}
					selected={[]}
					placeholder="Search members..."
					compact
				/>

				<div class="flex flex-col">
					{#each mockMembers as member}
						<div class="group flex w-full items-center rounded-md transition-colors duration-300">
							<button class="flex grow items-center gap-2" onclick={() => {}}>
								<div class="size-6 overflow-hidden rounded-full bg-gray-50 dark:bg-gray-600">
									<img
										src={member.iconURL}
										class="h-full w-full object-cover"
										alt="agent member icon"
									/>
								</div>
								<p class="truncate text-left text-sm font-light">
									{member.email}
								</p>
							</button>
							<button
								class="icon-button"
								onclick={() => (toDelete = member.email)}
								use:tooltip={'Remove member'}
							>
								<X class="size-4" />
							</button>
						</div>
					{/each}
				</div>
			</div>
		</CollapsePane>

		<CollapsePane
			classes={{
				header: 'pl-3 pr-5.5 py-2 border-surface3 border-b',
				content: 'p-3 border-b border-surface3'
			}}
			iconSize={4}
		>
			{#snippet header()}
				<p class="w-full text-left text-sm font-normal">ChatBot</p>
			{/snippet}
			<div class="flex flex-col gap-3">
				<div class="flex w-full items-center justify-between gap-4">
					<p class="flex grow text-sm">Enable ChatBot</p>
					<Toggle label="Toggle ChatBot" checked={!!share?.publicID} onChange={handleChange} />
				</div>

				{#if share?.publicID}
					<div
						class="dark:bg-surface2 flex w-full flex-col gap-2 rounded-xl bg-white p-3 shadow-sm"
					>
						<p class="text-xs text-gray-500">
							<b>Anyone with this link</b> can use this agent, which includes <b>any credentials</b>
							assigned to this agent.
						</p>
						<div class="flex gap-1">
							<CopyButton text={url} />
							<a href={url} class="overflow-hidden text-sm text-ellipsis hover:underline">{url}</a>
						</div>
					</div>
				{:else}
					<p class="text-xs text-gray-500">
						Enable ChatBot to allow anyone with the link to use this agent.
					</p>
				{/if}
			</div>
		</CollapsePane>

		<CollapsePane
			classes={{
				header: 'pl-3 pr-5.5 py-2 border-surface3 border-b',
				content: 'p-3 border-b border-surface3'
			}}
			iconSize={4}
		>
			{#snippet header()}
				<p class="w-full text-left text-sm font-normal">Agent Template</p>
			{/snippet}
			<div class="flex flex-col gap-3">
				<p class="text-xs text-gray-500">Under construction</p>
			</div>
		</CollapsePane>
	</div>
</CollapsePane>

<Confirm
	msg={`Remove ${toDelete} from your agent?`}
	show={!!toDelete}
	onsuccess={async () => {
		if (!toDelete) return;
		try {
			// TODO: remove member from project
		} finally {
			toDelete = '';
		}
	}}
	oncancel={() => (toDelete = '')}
/>
