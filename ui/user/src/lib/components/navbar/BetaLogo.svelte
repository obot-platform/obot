<script lang="ts">
	import { darkMode } from '$lib/stores';
	interface Props {
		chat?: boolean;
	}
	let { chat }: Props = $props();
</script>

<div class="flex flex-shrink-0">
	{#if darkMode.isDark}
		{#if chat}
			<img
				src="/user/images/obot-chat-logo-blue-white-text.svg"
				class="h-[43px] pl-[1px]"
				alt="Obot chat logo"
			/>
		{:else}
			<img src="/user/images/obot-logo-blue-white-text.svg" class="h-12" alt="Obot logo" />
		{/if}
	{:else if chat}
		<img
			src="/user/images/obot-chat-logo-blue-black-text.svg"
			class="h-[43px] pl-[1px]"
			alt="Obot chat logo"
		/>
	{:else}
		<img src="/user/images/obot-logo-blue-black-text.svg" class="h-12" alt="Obot logo" />
	{/if}
</div>
