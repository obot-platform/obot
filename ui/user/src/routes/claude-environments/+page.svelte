<script lang="ts">
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import TabLayout from '$lib/components/TabLayout.svelte';
	import ClaudeAccountManagement from '$lib/components/claude/ClaudeAccountManagement.svelte';
	import ClaudeConnectionForm from '$lib/components/claude/ClaudeConnectionForm.svelte';
	import ClaudeConnections from '$lib/components/claude/ClaudeConnections.svelte';
	import ClaudeSessions from '$lib/components/claude/ClaudeSessions.svelte';
	import BetaLogo from '$lib/components/navbar/BetaLogo.svelte';
	import { m } from '$lib/i18n';
	import type { ClaudeEnvironment } from '$lib/services';
	import { profile, version } from '$lib/stores';
	import { setUrlParamAndUpdateUrl } from '$lib/url';
	import { Plus } from '@lucide/svelte';

	const environmentId = $derived(page.url.searchParams.get('environment'));
	const showingForm = $derived(
		!!environmentId && version.current.engine !== 'docker' && !!profile.current.isAdmin?.()
	);
	const layoutTitle = $derived(
		!showingForm
			? m.nav_claude_environments()
			: environmentId === 'new'
				? m.hosted_agents_env_add()
				: m.hosted_agents_env_edit()
	);

	function openEnvironment(id: string) {
		setUrlParamAndUpdateUrl(page.url, 'environment', id);
	}

	function closeEnvironment() {
		setUrlParamAndUpdateUrl(page.url, 'environment', undefined);
	}
</script>

<svelte:head><title>Obot | {layoutTitle}</title></svelte:head>

{#if showingForm && environmentId}
	<Layout title={layoutTitle} showBackButton onBackButtonClick={closeEnvironment}>
		<div class="w-full">
			{#key environmentId}
				<ClaudeConnectionForm
					environment={environmentId}
					onCancel={closeEnvironment}
					onSubmit={closeEnvironment}
				/>
			{/key}
		</div>
	</Layout>
{:else}
	<TabLayout
		title={m.nav_claude_environments()}
		defaultView="environments"
		classes={{ childrenContainer: 'max-w-none' }}
		views={[
			{ label: m.nav_claude_environments(), value: 'environments', content: environments },
			...(version.current.engine === 'docker' || !version.current.claudeEnvironmentsEnabled
				? []
				: [
						{ label: m.hosted_agents_sessions(), value: 'sessions', content: sessions },
						...(profile.current.isAdmin?.()
							? [
									{
										label: m.hosted_agents_account_management(),
										value: 'accounts',
										content: accounts
									}
								]
							: [])
					])
		]}
		rightNavActions={navActions}
	/>
{/if}

{#snippet navActions(view: string)}
	{#if view === 'environments' && profile.current.isAdmin?.() && version.current.engine !== 'docker' && version.current.claudeEnvironmentsEnabled}
		<button
			class="btn btn-primary btn-block w-full text-sm md:w-52"
			id="add-catalog-entry-button"
			onclick={() => openEnvironment('new')}
		>
			<Plus class="size-4" />
			{m.hosted_agents_env_add()}
		</button>
	{/if}
{/snippet}

{#snippet environments()}
	{#if version.current.engine === 'docker'}
		<div class="w-full h-full flex justify-center items-center">
			<div class="paper w-full md:max-w-sm">
				<BetaLogo class="mx-auto" />
				<p class="text-xs font-light leading-normal">{m.hosted_agents_kubernetes_required()}</p>
				<p class="text-xs font-light leading-normal">
					{m.hosted_agents_kubernetes_docs_prefix()}<a
						href="https://docs.obot.ai/claude-environments/overview/"
						target="_blank"
						rel="external noopener noreferrer"
						class="link text-primary">{m.hosted_agents_kubernetes_docs_link()}</a
					>{m.hosted_agents_kubernetes_docs_suffix()}
				</p>
			</div>
		</div>
	{:else}
		<div class="w-full">
			<ClaudeConnections
				onEdit={(environment: ClaudeEnvironment) => openEnvironment(environment.id)}
			/>
		</div>
	{/if}
{/snippet}

{#snippet sessions()}
	<ClaudeSessions all={!!profile.current.isAdmin?.()} />
{/snippet}

{#snippet accounts()}
	<ClaudeAccountManagement />
{/snippet}
