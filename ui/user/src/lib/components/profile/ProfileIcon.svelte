<script lang="ts">
	import { m } from '$lib/i18n';
	import { profile } from '$lib/stores';
	import { ShieldUser } from '@lucide/svelte';
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
		alt={m.profile_alt()}
		referrerpolicy="no-referrer"
	/>
{:else if profile.current.isBootstrapUser?.()}
	<ShieldUser class="text-muted-content size-8 rounded-full" />
{:else}
	<div
		class="flex h-8 w-8 items-center justify-center rounded-full bg-base-300 dark:bg-base-200 text-white"
	>
		{initials}
	</div>
{/if}
