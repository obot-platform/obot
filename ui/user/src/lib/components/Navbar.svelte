<script lang="ts">
	import Profile from '$lib/components/navbar/Profile.svelte';
	import { fade } from 'svelte/transition';
	import type { Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';
	import BetaLogo from './navbar/BetaLogo.svelte';

	interface Props {
		leftContent?: Snippet;
		centerContent?: Snippet;
		class?: string;
		unauthorized?: boolean;
		hideProfileButton?: boolean;
		chat?: boolean;
		workflow?: boolean;
	}

	let {
		leftContent,
		centerContent,
		class: klass,
		unauthorized,
		hideProfileButton,
		chat,
		workflow
	}: Props = $props();
</script>

<nav class={twMerge('bg-background flex h-16 w-full items-center px-3', klass)} in:fade|global>
	<div class="flex w-full items-center justify-between">
		{#if leftContent}
			{@render leftContent()}
		{:else}
			<BetaLogo {chat} {workflow} />
		{/if}
		<div class="flex grow items-center justify-center">
			{#if centerContent}
				{@render centerContent()}
			{/if}
		</div>
		{#if !unauthorized && !hideProfileButton}
			<div class="flex h-16 items-center">
				<Profile />
			</div>
		{/if}
	</div>
</nav>
