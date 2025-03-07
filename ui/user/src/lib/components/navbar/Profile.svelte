<script lang="ts">
	import ProfileIcon from '$lib/components/profile/ProfileIcon.svelte';
	import { profile } from '$lib/stores';
	import Menu from '$lib/components/navbar/Menu.svelte';
</script>

<Menu title={profile.current.getDisplayName?.() || 'Anonymous'}>
	{#snippet icon()}
		<ProfileIcon />
	{/snippet}
	{#snippet body()}
		<div class="flex flex-col gap-2 py-2">
			{#if profile.current.role === 1}
				<a href="/admin/" rel="external" role="menuitem" class="text-red-400">Admin</a>
			{/if}
			{#if profile.current.email}
				<a href="/oauth2/sign_out?rd=/" rel="external" role="menuitem">Sign out</a>
			{/if}
		</div>
	{/snippet}
</Menu>
