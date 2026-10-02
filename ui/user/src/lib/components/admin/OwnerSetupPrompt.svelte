<script lang="ts">
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import type { AuthProvider } from '$lib/services';

	interface Props {
		isLocalSetup: boolean;
		localUserEmail?: string;
		explicitOwners?: string[];
		provider?: AuthProvider;
		tempLoginUrl: string;
		onManageLocalUsers?: () => void;
		showTitle?: boolean;
	}

	const {
		isLocalSetup,
		localUserEmail,
		explicitOwners = [],
		provider,
		tempLoginUrl,
		onManageLocalUsers,
		showTitle = true
	}: Props = $props();
</script>

{#if showTitle}
	<h3 class="mb-4 text-lg font-semibold">{m.admin_misc_next_step_owner_setup()}</h3>
{/if}

<div class="flex flex-col gap-2">
	{#if isLocalSetup}
		<p>
			{#if localUserEmail}
				{m.admin_misc_finish_setup_signing_in_as_prefix()}
				<b>{localUserEmail}</b>{m.admin_misc_finish_setup_signing_in_as_suffix()}
			{:else}
				{m.admin_misc_finish_setup_local_accounts()}
			{/if}
		</p>
		<p>
			{m.admin_misc_account_becomes_owner_prefix()}
			<b>{m.admin_misc_account_becomes_owner_word()}</b>
			{m.admin_misc_account_becomes_owner_suffix()}
		</p>
	{:else if explicitOwners.length > 0}
		<p>{m.admin_misc_continue_with_owner_account()}</p>
		<p>{m.admin_misc_explicit_owners_listed()}</p>
		<ul class="list-disc px-8">
			{#each explicitOwners as owner (owner)}
				<li>{owner}</li>
			{/each}
		</ul>
		<p>
			{m.admin_misc_login_as_explicit_owner()}
		</p>
		<p>
			{m.admin_misc_login_different_account()}
		</p>
	{:else}
		<p>
			{m.admin_misc_setup_initial_owner()}
		</p>
	{/if}

	<div class="my-4 flex flex-col gap-2">
		{#if tempLoginUrl}
			<a class="btn btn-secondary w-full" href={tempLoginUrl} rel="external">
				{#if provider?.icon}
					<img
						class="bg-base-100 h-6 w-6 rounded-full p-1 dark:bg-gray-600"
						src={provider.icon}
						alt={provider.name}
					/>
				{/if}
				<span class="text-center text-sm font-light">
					{#if isLocalSetup && localUserEmail}
						{m.admin_misc_sign_in_as({ email: localUserEmail })}
					{:else}
						{m.login_continue_with({ provider: provider?.name ?? '' })}
					{/if}
				</span>
			</a>
		{:else}
			<div class="w-full justify-center items-center flex">
				<Loading class="size-4" />
			</div>
		{/if}
		{#if isLocalSetup && onManageLocalUsers}
			<p class="text-muted-content text-center text-xs font-light">
				{m.admin_misc_change_owner_account()}
				<button type="button" class="text-link underline" onclick={onManageLocalUsers}>
					{m.admin_misc_click_here()}
				</button>
			</p>
		{/if}
	</div>
</div>
