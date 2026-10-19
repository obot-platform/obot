<script lang="ts">
	import { resolve } from '$app/paths';
	import { m } from '$lib/i18n';
	import { UserService, type ClaudeEnvironment } from '$lib/services';

	let { environment, onsaved }: { environment: ClaudeEnvironment; onsaved: () => Promise<void> } =
		$props();
	let enabled = $state(false);
	let busy = $state(false);
	let error = $state('');
	let saved = $state(false);
	$effect(() => {
		enabled = environment.claudeTagEnabled;
	});

	async function save(event: SubmitEvent) {
		event.preventDefault();
		busy = true;
		error = '';
		saved = false;
		try {
			await UserService.updateClaudeEnvironmentTag(
				environment.id,
				environment.claudeTagUserID ? enabled : true
			);
			await onsaved();
			saved = true;
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
		} finally {
			busy = false;
		}
	}
</script>

<form
	class="paper block space-y-3"
	onsubmit={save}
	aria-label={m.hosted_agents_tag_for({ name: environment.displayName })}
>
	<h3 class="font-semibold">{m.hosted_agents_tag_heading()}</h3>
	<p class="text-sm text-muted-content">
		{m.hosted_agents_tag_help()}
	</p>
	<p id={`claude-tag-access-${environment.id}`} class="text-sm text-muted-content">
		{m.hosted_agents_claude_tag_profile_required()}
	</p>
	{#if environment.claudeTagUserID}
		<p class="text-sm">
			{m.hosted_agents_tag_service_user()}
			<strong>{m.hosted_agents_tag_heading()} — {environment.displayName}</strong>
			(ID:
			<code>{environment.claudeTagUserID}</code>)
		</p>
		<p class="text-sm text-muted-content">
			{m.hosted_agents_tag_grant()}
			<a class="link" href={resolve('/vmcps')}>{m.hosted_agents_tag_vmcp_profiles()}</a>
		</p>
		<label class="flex items-center gap-2 text-sm"
			><input
				class="checkbox checkbox-sm"
				type="checkbox"
				aria-describedby={`claude-tag-access-${environment.id}`}
				bind:checked={enabled}
				disabled={busy}
			/>{m.hosted_agents_tag_enable()}</label
		>
		<p class="text-sm text-muted-content">
			{m.hosted_agents_tag_disable_help()}
		</p>
	{/if}
	{#if error}<p role="alert" class="text-error">{error}</p>{/if}
	{#if saved}<p role="status" class="text-success">
			{m.hosted_agents_tag_saved()}
		</p>{/if}
	<button class="btn btn-primary btn-sm" type="submit" disabled={busy}
		>{busy
			? m.hosted_agents_map_saving()
			: environment.claudeTagUserID
				? m.hosted_agents_tag_save()
				: m.hosted_agents_tag_create()}</button
	>
</form>
