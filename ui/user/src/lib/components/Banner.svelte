<script lang="ts">
	import { profile } from '$lib/stores';
	import { X } from 'lucide-svelte';
	import { slide } from 'svelte/transition';

	let banner = $state<HTMLDivElement>();
	let showBanner = $state(false);
	const A_WEEK_AGO = new Date(Date.now() - 7 * 24 * 60 * 60 * 1000).getTime();
	const BANNER_KEY = 'obot-upgrade-banner-viewed';

	$effect(() => {
		if (
			localStorage.getItem(BANNER_KEY) !== 'true' &&
			profile.current.loaded &&
			profile.current.created &&
			new Date(profile.current.created).getTime() < A_WEEK_AGO // account is older than a week
		) {
			showBanner = true;
		}
	});
</script>

{#if showBanner}
	<div
		bind:this={banner}
		class="flex w-full items-center justify-center bg-blue-500 transition-all duration-300"
		out:slide
	>
		<div class="flex w-full max-w-(--breakpoint-xl) items-center justify-between gap-4 px-4 py-2">
			<div class="flex flex-col gap-0.5">
				<p class="text-xs font-semibold text-white">
					Things look different? We just released a big upgrade!
				</p>
				<p class="text-xs text-white">
					Unfortunately, in the name of progress, older agents may no longer work. If that's the
					case for you, please delete and recreate your agent. Thanks for your patience while we
					make Obot awesome!
				</p>
			</div>
			<button
				onclick={() => {
					localStorage.setItem(BANNER_KEY, 'true');
					showBanner = false;
				}}
				class="icon-button-small rounded-full hover:bg-blue-600 dark:hover:bg-blue-600"
			>
				<X class="size-4 text-white" />
			</button>
		</div>
	</div>
{/if}
