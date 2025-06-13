<script lang="ts">
	import { clickOutside } from '$lib/actions/clickoutside';
	import { AdminService } from '$lib/services';
	import { Role, type OrgUser } from '$lib/services/admin/types';
	import { responsive } from '$lib/stores';
	import { Check, ChevronRight, LoaderCircle, User, Users, X } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';
	import Search from '../Search.svelte';

	interface Props {
		onAdd: (users: OrgUser[]) => void;
	}

	let addUserGroupDialog = $state<HTMLDialogElement>();
	let fetchingUsers = $state<Promise<OrgUser[]>>();
	let searchUsers = $state('');
	let selectedUsers = $state<OrgUser[]>([]);
	let selectedUsersMap = $derived(new Set(selectedUsers.map((user) => user.id)));

	export function show() {
		fetchingUsers = AdminService.listUsers();
		addUserGroupDialog?.showModal();
	}

	let { onAdd }: Props = $props();
</script>

<dialog
	bind:this={addUserGroupDialog}
	use:clickOutside={() => addUserGroupDialog?.close()}
	class="h-full max-h-screen max-w-full overflow-visible md:h-[500px] md:min-w-md"
	class:mobile-screen-dialog={responsive.isMobile}
>
	<div class="flex h-full flex-col justify-between gap-4 py-4">
		<h3 class="default-dialog-title px-4" class:default-dialog-mobile-title={responsive.isMobile}>
			Add User/Group
			<button
				class:mobile-header-button={responsive.isMobile}
				onclick={() => addUserGroupDialog?.close()}
				class="icon-button"
			>
				{#if responsive.isMobile}
					<ChevronRight class="size-6" />
				{:else}
					<X class="size-5" />
				{/if}
			</button>
		</h3>
		<div class="default-scrollbar-thin flex grow flex-col gap-4 overflow-y-scroll pt-1">
			{#await fetchingUsers}
				<div class="flex grow items-center justify-center">
					<LoaderCircle class="size-6 animate-spin" />
				</div>
			{:then users}
				{@const filteredUsers =
					searchUsers.length > 0
						? (users?.filter(
								(user) =>
									user.email.toLowerCase().includes(searchUsers.toLowerCase()) ||
									user.username.toLowerCase().includes(searchUsers.toLowerCase())
							) ?? [])
						: (users ?? [])}
				<div class="px-4">
					<Search
						class="dark:bg-surface1 dark:border-surface3 shadow-inner dark:border"
						onChange={(val) => (searchUsers = val)}
						placeholder="Search by name or email..."
					/>
				</div>
				<div class="flex flex-col">
					{#each filteredUsers ?? [] as user (user.id)}
						<button
							class={twMerge(
								'dark:hover:bg-surface1 hover:bg-surface2 flex items-center gap-2 px-4 py-2 text-left',
								selectedUsersMap.has(user.id) && 'dark:bg-gray-920 bg-gray-50'
							)}
							onclick={() => {
								if (selectedUsersMap.has(user.id)) {
									const index = selectedUsers.findIndex((u) => u.id === user.id);
									if (index !== -1) {
										selectedUsers.splice(index, 1);
									}
								} else {
									selectedUsers.push(user);
									selectedUsersMap.add(user.id);
								}
							}}
						>
							<img src={user.iconURL} alt={user.username} class="size-10 rounded-full" />
							<div class="flex grow flex-col">
								<p>{user.email}</p>
								<p class="font-light text-gray-400 dark:text-gray-600">
									{user.role === Role.ADMIN ? 'Admin' : 'User'}
								</p>
							</div>
							<div class="flex items-center justify-center">
								{#if selectedUsersMap.has(user.id)}
									<Check class="size-6 text-blue-500" />
								{/if}
							</div>
						</button>
					{/each}
				</div>
			{/await}
		</div>
		<div class="flex w-full flex-col justify-between gap-4 p-4 pb-0 md:flex-row">
			<div class="flex items-center gap-1 font-light">
				{#if selectedUsers.length > 0}
					{#if selectedUsers.length === 1}
						<User class="size-4" />
					{:else}
						<Users class="size-4" />
					{/if}
					{selectedUsers.length} Selected
				{/if}
			</div>
			<div class="flex items-center gap-4">
				<button class="button w-full md:w-fit" onclick={() => addUserGroupDialog?.close()}>
					Cancel
				</button>
				<button class="button-primary w-full md:w-fit" onclick={() => {}}> Confirm </button>
			</div>
		</div>
	</div>
</dialog>
