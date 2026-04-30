<script lang="ts">
	import ProfileIcon from '$lib/components/profile/ProfileIcon.svelte';
	import type { Message } from '$lib/services';
	import { CircleAlert } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		msg: Message;
		class?: string;
	}

	let { msg, class: klass }: Props = $props();
</script>

{#if !msg.icon}
	<!-- Nothing -->
{:else if msg.icon === 'Pencil' || msg.icon === 'Assistant'}
	<!-- Nothing -->
{:else}
	<div class={twMerge('shrink-0', klass)}>
		{#if msg.icon === 'Profile'}
			<ProfileIcon />
		{:else if msg.icon === 'Error'}
			<CircleAlert class="size-4 text-error md:size-6" />
		{:else}
			<img
				class="text-primary size-4 rounded-md bg-base-300 p-1 md:size-6"
				src={msg.icon}
				alt="message icon"
			/>
		{/if}
	</div>
{/if}
