<script lang="ts">
	import { BOOTSTRAP_USER_ID } from '$lib/constants';
	import { profile } from '$lib/stores';
	import { ShieldUser } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		class?: string;
	}

	let { class: klass }: Props = $props();

	let initials = $state('?');

	$effect(() => {
		if (profile.current.email) {
			const parts = profile.current.email.split('@')[0].split(/[.-]/);
			let newInitials = parts[0].charAt(0).toUpperCase();
			if (parts.length > 1) {
				newInitials += parts[parts.length - 1].charAt(0).toUpperCase();
			}
			if (newInitials !== initials) {
				initials = newInitials;
			}
		}
	});
</script>

{#if profile.current.iconURL}
	<img
		class={twMerge('size-8 rounded-full', klass)}
		src={profile.current.iconURL}
		alt="profile"
		referrerpolicy="no-referrer"
	/>
{:else if profile.current.username === BOOTSTRAP_USER_ID}
	<ShieldUser class="size-8 rounded-full text-gray-400 dark:text-gray-600" />
{:else}
	<div class="flex h-8 w-8 items-center justify-center rounded-full bg-gray-600 text-white">
		{initials}
	</div>
{/if}
