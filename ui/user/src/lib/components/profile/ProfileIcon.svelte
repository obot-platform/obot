<script lang="ts">
	import { profile } from '$lib/stores';

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
		class="h-8 w-8 rounded-full"
		src={profile.current.iconURL}
		alt="profile"
		referrerpolicy="no-referrer"
	/>
{:else}
	<div class="flex h-8 w-8 items-center justify-center rounded-full bg-gray-600 text-white">
		{initials}
	</div>
{/if}
