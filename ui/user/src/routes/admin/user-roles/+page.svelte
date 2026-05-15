<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import Loading from '$lib/icons/Loading.svelte';
	import { AdminService } from '$lib/services';
	import { userRoleOptions } from '$lib/services/admin/constants';
	import { Role } from '$lib/services/admin/types';
	import { profile } from '$lib/stores/index.js';
	import { untrack } from 'svelte';
	import { fade } from 'svelte/transition';

	const duration = PAGE_TRANSITION_DURATION;

	let { data } = $props();
	let showSaved = $state(false);
	let baseDefaultRole = $state(untrack(() => data.defaultUsersRole ?? Role.BASIC));
	let prevBaseDefaultRole = $state(untrack(() => data.defaultUsersRole ?? Role.BASIC));
	let saving = $state(false);
	let timeout = $state<ReturnType<typeof setTimeout>>();

	async function handleSave() {
		if (timeout) {
			clearTimeout(timeout);
		}

		saving = true;
		await AdminService.updateDefaultUsersRoleSettings(baseDefaultRole);
		prevBaseDefaultRole = baseDefaultRole;
		saving = false;
		showSaved = true;
		timeout = setTimeout(() => {
			showSaved = false;
		}, 3000);
	}

	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
</script>

<Layout classes={{ container: 'pb-0' }} title="User Roles">
	<div
		class="mb-4 flex h-dvh min-h-full flex-col gap-8"
		in:fade={{ duration }}
		out:fade={{ duration }}
	>
		<div class="paper">
			<div class="flex gap-6">
				<div class="flex grow flex-col gap-4">
					<div class="flex flex-col gap-1">
						<h4 class="text-lg font-semibold">Default User Role</h4>
						<p class="text-muted-content text-sm font-light">
							Set the initial default role for all new users when they first log into the system.
							User roles can be changed individually from the "Users" page.
						</p>

						<div class="mt-4 flex flex-col gap-4">
							{#each userRoleOptions as role (role.id)}
								<label class="flex items-center gap-4" for={`role-${role.id}`}>
									<input
										type="radio"
										name="role"
										id={`role-${role.id}`}
										value={role.id}
										bind:group={baseDefaultRole}
										disabled={isAdminReadonly}
									/>
									<div class="flex flex-col">
										<p class="text-sm font-medium">{role.label}</p>
										<p class="text-muted-content text-sm font-light">{role.description}</p>
									</div>
								</label>
							{/each}
						</div>
					</div>
				</div>
			</div>
		</div>

		<div class="flex grow"></div>

		{#if !isAdminReadonly}
			<div
				class="bg-base-200 dark:bg-base-100 sticky bottom-0 left-0 flex w-[calc(100%+2em)] -translate-x-4 justify-end gap-4 p-4 md:w-[calc(100%+4em)] md:-translate-x-8 md:px-8"
			>
				{#if showSaved}
					<span
						in:fade={{ duration: 200 }}
						class="text-muted-content flex min-h-10 items-center px-4 text-sm font-extralight"
					>
						Your changes have been saved.
					</span>
				{/if}

				<button
					class="btn btn-secondary hover:bg-base-400 flex items-center gap-1 bg-transparent"
					onclick={() => {
						baseDefaultRole = prevBaseDefaultRole;
					}}
				>
					Reset
				</button>
				<button
					class="btn btn-primary flex items-center gap-1"
					disabled={saving}
					onclick={handleSave}
				>
					{#if saving}
						<Loading class="size-4" />
					{:else}
						Save
					{/if}
				</button>
			</div>
		{/if}
	</div>
</Layout>

<svelte:head>
	<title>Obot | User Roles</title>
</svelte:head>
