<script lang="ts">
	import ProfileIcon from '$lib/components/profile/ProfileIcon.svelte';
	import type { Message } from '$lib/services';
	import { Pencil } from 'lucide-svelte/icons';
	import { AlertCircle } from 'lucide-svelte';

	interface Props {
		msg: Message;
	}

	let { msg }: Props = $props();
</script>

{#if !msg.icon}
	<!-- Nothing -->
{:else if msg.icon === 'Assistant'}
	<!-- Nothing -->
{:else}
	<div class="shrink-0">
		{#if msg.icon === 'Pencil'}
			<Pencil class="size-4 md:size-6" />
		{:else if msg.icon === 'Assistant'}
			<!-- Nothing -->
		{:else if msg.icon === 'Profile'}
			<ProfileIcon />
		{:else if msg.icon === 'Error'}
			<AlertCircle class="size-4 text-red-500 md:size-6" />
		{:else}
			<img
				class="text-blue size-4 rounded-md bg-gray-100 p-1 md:size-6"
				src={msg.icon}
				alt="message icon"
			/>
		{/if}
	</div>
{/if}
