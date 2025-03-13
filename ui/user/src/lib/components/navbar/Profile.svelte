<script lang="ts">
	import ProfileIcon from '$lib/components/profile/ProfileIcon.svelte';
	import { profile } from '$lib/stores';
	import Menu from '$lib/components/navbar/Menu.svelte';
	import { Moon, Sun } from 'lucide-svelte/icons';
	import { darkMode } from '$lib/stores';
</script>

<Menu title={profile.current.getDisplayName?.() || 'Anonymous'}>
	{#snippet icon()}
		<ProfileIcon />
	{/snippet}
	{#snippet body()}
		<div class="flex flex-col gap-2 py-2">
			<button
				type="button"
				onclick={() => {
					darkMode.setDark(!darkMode.isDark);
				}}
				role="menuitem"
				class="icon-button"
			>
				{#if darkMode.isDark}
					<Sun class="h-5 w-5" />
				{:else}
					<Moon class="h-5 w-5" />
				{/if}
			</button>
			{#if profile.current.role === 1}
				<a href="/admin/" rel="external" role="menuitem" class="icon-button" style="color: #f87171;"
					>Admin</a
				>
			{/if}
			{#if profile.current.email}
				<a href="/oauth2/sign_out?rd=/" rel="external" role="menuitem" class="icon-button"
					>Sign out</a
				>
			{/if}
		</div>
	{/snippet}
</Menu>
