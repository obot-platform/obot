<script>
	import { darkMode } from '$lib/stores';
</script>

<div class="flex flex-shrink-0">
	{#if darkMode.isDark}
		<img src="/user/images/obot-logo-blue-white-text.svg" class="h-12" alt="Obot logo" />
	{:else}
		<img src="/user/images/obot-logo-blue-black-text.svg" class="h-12" alt="Obot logo" />
	{/if}
	<span
		class="mt-auto mb-1 ml-1 h-fit rounded-full border-2 border-blue-400 px-1.5 py-[1px] text-[10px] font-bold text-blue-400 dark:border-blue-400 dark:text-blue-400"
	>
		BETA
	</span>
</div>
