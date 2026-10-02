<script lang="ts">
	import { resolve } from '$app/paths';
	import { m } from '$lib/i18n';
	import { UserService } from '$lib/services';
	import { profile } from '$lib/stores';
	import { HatGlasses } from '@lucide/svelte';

	let { agent } = $props();

	// Detect impersonation by comparing agent owner to current user.
	const impersonating = $derived(
		!!profile.current?.id && !!agent.userID && agent.userID !== profile.current.id
	);

	let ownerEmail = $state('');
	$effect(() => {
		if (impersonating && agent.userID) {
			UserService.getUser(agent.userID)
				.then((owner) => {
					ownerEmail = owner.email || owner.username || agent.userID;
				})
				.catch(() => {
					ownerEmail = agent.userID;
				});
		}
	});
</script>

{#if impersonating}
	<div
		class="sticky top-0 left-0 z-50 bg-primary flex flex-col items-center justify-center gap-2 px-4 py-3 text-sm font-light text-white md:flex-row"
	>
		<p class="text-center md:text-left">
			<HatGlasses class="inline-block size-5" />
			<span class="font-semibold">{m.chat_impersonate_caution()}</span>
			{m.chat_impersonate_prefix()}
			<span class="font-semibold">{ownerEmail}{m.chat_impersonate_suffix()}</span> <br />
		</p>
		<a href={resolve('/admin/agents')} class="btn btn-sm text-base-content font-normal"
			>{m.chat_stop_impersonating()}</a
		>
	</div>
{/if}
