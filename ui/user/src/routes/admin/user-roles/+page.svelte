<script lang="ts">
	import { fade } from 'svelte/transition';
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { LoaderCircle } from 'lucide-svelte';
	import { Role } from '$lib/services/admin/types';
	import { userRoleOptions } from '$lib/services/admin/constants';
	import { AdminService } from '$lib/services';

	const duration = PAGE_TRANSITION_DURATION;

	let { data } = $props();
	let showSaved = $state(false);
	let baseDefaultRole = $state(data.defaultUsersRole ?? Role.BASIC);
	let prevBaseDefaultRole = $state(data.defaultUsersRole ?? Role.BASIC);
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
</script>

<Layout classes={{ container: 'pb-0' }}>
	<div
		class="my-4 flex h-dvh min-h-full flex-col gap-8"
		in:fade={{ duration }}
		out:fade={{ duration }}
	>
		<h1 class="text-2xl font-semibold">User Roles</h1>

		<div
			class="dark:bg-surface1 dark:border-surface3 flex h-fit w-full flex-col gap-4 rounded-lg border border-transparent bg-white p-6 shadow-sm"
		>
			<div class="flex gap-6">
				<div class="flex grow flex-col gap-4">
					<div class="flex flex-col gap-1">
						<h4 class="text-lg font-semibold">Default User Role</h4>
						<p class="text-sm font-light text-gray-500">
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
									/>
									<div class="flex flex-col">
										<p class="text-sm font-medium">{role.label}</p>
										<p class="text-sm font-light text-gray-500">{role.description}</p>
									</div>
								</label>
							{/each}
						</div>
					</div>
				</div>
			</div>
		</div>

		<div class="flex grow"></div>

		<div
			class="bg-surface1 sticky bottom-0 left-0 flex w-[calc(100%+2em)] -translate-x-4 justify-end gap-4 p-4 md:w-[calc(100%+4em)] md:-translate-x-8 md:px-8 dark:bg-black"
		>
			{#if showSaved}
				<span
					in:fade={{ duration: 200 }}
					class="flex min-h-10 items-center px-4 text-sm font-extralight text-gray-500"
				>
					Your changes have been saved.
				</span>
			{/if}

			<button
				class="button hover:bg-surface3 flex items-center gap-1 bg-transparent"
				onclick={() => {
					baseDefaultRole = prevBaseDefaultRole;
				}}
			>
				Reset
			</button>
			<button class="button-primary flex items-center gap-1" disabled={saving} onclick={handleSave}>
				{#if saving}
					<LoaderCircle class="size-4 animate-spin" />
				{:else}
					Save
				{/if}
			</button>
		</div>
	</div>
</Layout>

<svelte:head>
	<title>Obot | User Roles</title>
</svelte:head>
