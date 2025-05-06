<script lang="ts">
	import { Crown, Plus, Trash2 } from 'lucide-svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { openSidebarConfig, getLayout } from '$lib/context/layout.svelte';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import { browser } from '$app/environment';
	import { ChatService, type Project, type ProjectShare, type ProjectMember } from '$lib/services';
	import { profile } from '$lib/stores';
	import { HELPER_TEXTS } from '$lib/context/helperMode.svelte';

	let toDelete = $state('');
	let ownerID = $state<string>('');
	let isOwnerOrAdmin = $derived(profile.current.id === ownerID || profile.current.role === 1);

	interface Props {
		project: Project;
	}

	let { project }: Props = $props();
	let members = $state<ProjectMember[]>([]);
	let share = $state<ProjectShare>();
	let url = $derived(
		browser && share?.publicID
			? `${window.location.protocol}//${window.location.host}/s/${share.publicID}`
			: ''
	);
	const layout = getLayout();

	async function loadMembers() {
		members = await ChatService.listProjectMembers(project.assistantID, project.id);
	}

	async function deleteMember(memberId: string) {
		if (!isOwnerOrAdmin) return;
		await ChatService.deleteProjectMember(project.assistantID, project.id, memberId);
		await loadMembers();
	}

	async function updateShare() {
		share = await ChatService.getProjectShare(project.assistantID, project.id);
	}

	function manageInvitations() {
		if (!isOwnerOrAdmin) return;
		openSidebarConfig(layout, 'invitations');
	}

	$effect(() => {
		if (project) {
			ownerID = project.userID;
			updateShare();
			loadMembers();
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

<CollapsePane
	classes={{ header: 'pl-3 py-2 text-md', content: 'p-0' }}
	iconSize={5}
	header="Sharing"
	helpText={HELPER_TEXTS.sharing}
>
	<div class="flex flex-col">
		<CollapsePane
			classes={{
				header: 'pl-3 pr-5.5 py-2 border-surface3 border-b',
				content: 'p-3 border-b border-surface3',
				headerText: 'text-sm font-normal'
			}}
			iconSize={4}
			header="Members"
			helpText={HELPER_TEXTS.members}
		>
			<div class="flex flex-col gap-2 text-sm">
				<div class="flex flex-col">
					<div class="mb-2 flex items-center justify-between">
						<span class="text-sm font-medium">Project Members</span>
						{#if isOwnerOrAdmin}
							<div class="flex gap-2">
								<button
									class="bg-surface3 hover:bg-surface4 rounded-full p-1 transition-colors"
									onclick={manageInvitations}
									use:tooltip={'Manage invitations'}
								>
									<Plus class="size-4" />
								</button>
							</div>
						{/if}
					</div>
					{#each members as member}
						<div
							class="group flex h-[36px] w-full items-center rounded-md transition-colors duration-300"
						>
							<div class="flex grow items-center gap-2">
								<div class="size-6 overflow-hidden rounded-full bg-gray-50 dark:bg-gray-600">
									<img
										src={member.iconURL}
										class="h-full w-full object-cover"
										alt="agent member icon"
										referrerpolicy="no-referrer"
									/>
								</div>
								<p class="truncate text-left text-sm font-light">
									{member.email}
								</p>
								{#if member.isOwner}
									<span use:tooltip={'Project Owner'}>
										<Crown class="size-4" />
									</span>
								{/if}
							</div>
							{#if isOwnerOrAdmin && profile.current.email !== member.email && !member.isOwner}
								<button
									class="bg-surface3 hover:bg-surface4 rounded-full p-1 transition-colors"
									onclick={() => (toDelete = member.email)}
									use:tooltip={'Remove member'}
								>
									<Trash2 class="size-4" />
								</button>
							{/if}
						</div>
					{/each}
				</div>
			</div>
		</CollapsePane>

		<CollapsePane
			classes={{
				header: 'pl-3 pr-5.5 py-2 border-surface3 border-b',
				content: 'p-3 border-b border-surface3',
				headerText: 'text-sm font-normal'
			}}
			iconSize={4}
			header="ChatBot"
			helpText={HELPER_TEXTS.chatbot}
		>
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
				content: 'p-3 border-b border-surface3',
				headerText: 'text-sm font-normal'
			}}
			iconSize={4}
			header="Agent Template"
			helpText={HELPER_TEXTS.agentTemplate}
		>
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
			const memberToDelete = members.find((m) => m.email === toDelete);
			if (memberToDelete) {
				await deleteMember(memberToDelete.userID);
			}
		} finally {
			toDelete = '';
		}
	}}
	oncancel={() => (toDelete = '')}
/>
