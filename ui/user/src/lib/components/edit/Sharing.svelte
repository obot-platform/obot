<script lang="ts">
	import { fade } from 'svelte/transition';
	import { Crown, Plus, Trash2 } from 'lucide-svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { ChatService, type Project, type ProjectMember } from '$lib/services';
	import { profile } from '$lib/stores';
	import { HELPER_TEXTS } from '$lib/context/helperMode.svelte';
	import { openSidebarConfig, getLayout } from '$lib/context/chatLayout.svelte';

	let toDelete = $state('');
	let ownerID = $state<string>('');
	let isOwnerOrAdmin = $derived(profile.current.id === ownerID || profile.current.isAdmin?.());

	interface Props {
		project: Project;
	}

	let { project }: Props = $props();
	let members = $state<ProjectMember[]>([]);
	const layout = getLayout();

	async function loadMembers() {
		members = await ChatService.listProjectMembers(project.assistantID, project.id);
	}

	async function deleteMember(memberId: string) {
		if (!isOwnerOrAdmin) return;
		await ChatService.deleteProjectMember(project.assistantID, project.id, memberId);
		await loadMembers();
	}

	function manageInvitations() {
		if (!isOwnerOrAdmin) return;
		openSidebarConfig(layout, 'invitations');
	}

	function openChatbotConfig() {
		openSidebarConfig(layout, 'chatbot');
	}

	$effect(() => {
		if (project) {
			ownerID = project.userID;
			loadMembers();
		}
	});
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
				content: 'p-3 border-b border-surface3 overflow-x-hidden',
				headerText: 'text-sm font-normal'
			}}
			iconSize={4}
			header="ChatBot"
			helpText={HELPER_TEXTS.chatbot}
		>
			<div class="flex flex-col gap-3">
				<p class="text-xs text-gray-500">
					Configure ChatBot to produce a link that allows anyone to use this project in a read-only
					mode.
				</p>
				<div class="mt-2 flex justify-end" in:fade>
					<button
						class="button flex cursor-pointer items-center justify-end gap-1 text-xs"
						onclick={openChatbotConfig}
					>
						<span>Configure ChatBot</span>
					</button>
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
			header="Members"
			helpText={HELPER_TEXTS.members}
		>
			<div class="flex flex-col gap-2 text-sm">
				<div class="flex flex-col gap-1">
					<div class="flex items-center justify-between">
						<span class="text-sm font-medium">Project Members</span>
						{#if isOwnerOrAdmin}
							<div class="flex gap-2">
								<button
									class="icon-button"
									onclick={manageInvitations}
									use:tooltip={'Manage invitations'}
								>
									<Plus class="size-4" />
								</button>
							</div>
						{/if}
					</div>
					{#each members as member (member.userID)}
						<div
							class="group flex min-h-9 w-full items-center rounded-md transition-colors duration-300"
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
									class="icon-button"
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
	</div>
</CollapsePane>

<Confirm
	msg={`Remove ${toDelete} from your project?`}
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
