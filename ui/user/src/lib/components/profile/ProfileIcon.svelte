<script lang="ts">
	import { profile } from '$lib/stores';
	import { HatGlasses, ShieldUser } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		class?: string;
		impersonating?: boolean;
	}

	let { class: klass, impersonating }: Props = $props();

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

{#if impersonating}
	<div class={twMerge('relative size-8', klass)}>
		<div
			class="bg-primary absolute top-1/2 left-1/2 z-10 flex size-full -translate-x-1/2 -translate-y-1/2 items-center justify-center rounded-full opacity-65"
		>
			<HatGlasses class="size-6 text-white" />
		</div>
		{@render profileImage()}
	</div>
{:else}
	{@render profileImage()}
{/if}

{#snippet profileImage()}
	{#if profile.current.iconURL}
		<img
			class={twMerge('size-8 rounded-full', klass)}
			src={profile.current.iconURL}
			alt="profile"
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
{/snippet}
