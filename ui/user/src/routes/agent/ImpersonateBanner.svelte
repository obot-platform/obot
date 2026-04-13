<script lang="ts">
	import { resolve } from '$app/paths';
	import { AdminService } from '$lib/services';
	import { profile } from '$lib/stores';
	import { HatGlasses } from 'lucide-svelte';

	let { agent } = $props();

	// Detect impersonation by comparing agent owner to current user.
	const impersonating = $derived(
		!!profile.current?.id && !!agent.userID && agent.userID !== profile.current.id
	);

	let ownerEmail = $state('');
	$effect(() => {
		if (impersonating && agent.userID) {
			AdminService.getUser(agent.userID)
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
			<HatGlasses class="inline-block size-5" /> <span class="font-semibold">CAUTION!</span> You are
			currently impersonating <span class="font-semibold">{ownerEmail}.</span> <br />
		</p>
		<a href={resolve('/admin/agents')} class="button text-on-background text-xs font-normal"
			>Stop Impersonating</a
		>
	</div>
{/if}
