<script lang="ts">
	import { browser } from '$app/environment';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import CopyField from '../CopyField.svelte';
	import { RefreshCw } from '@lucide/svelte';

	let {
		url,
		onComplete,
		loading = false
	}: { url: string; onComplete?: () => void | Promise<void>; loading?: boolean } = $props();
	let root = $state<HTMLDivElement>();
	let checking = $state(false);
	let checkFailed = $state(false);
	const id = $props.id();
	const quote = (value: string) => "'" + value.replaceAll("'", "'\"'\"'") + "'";
	let loginURL = $derived(
		browser && url.startsWith('/') ? new URL(url, window.location.origin).href : url
	);
	let command = $derived(`obot mcp login --url ${quote(loginURL)}`);

	// Watching the existing attempt is read-only. Re-running the owning flow while
	// it is pending would generate new OAuth states and replace the copied command.
	$effect(() => {
		const attemptURL = loginURL;
		if (!root || !onComplete) return;
		const dialog = root.closest('dialog');
		let stopped = false;
		let settled = false;
		let timer: ReturnType<typeof setTimeout>;
		let controller: AbortController | undefined;
		const active = () => !stopped && (!dialog || dialog.open);
		const schedule = () => {
			clearTimeout(timer);
			if (active() && !settled) timer = setTimeout(checkAttempt, 2000);
		};
		async function checkAttempt() {
			if (!active()) return;
			if (document.visibilityState !== 'visible' || loading || checking) {
				schedule();
				return;
			}
			const request = new AbortController();
			controller = request;
			try {
				const response = await fetch(attemptURL, { signal: request.signal, cache: 'no-store' });
				if (request.signal.aborted || !active() || document.visibilityState !== 'visible') return;
				if (response.status === 400) {
					// Consumed, failed, or expired is not proof of success. The owner
					// verifies authentication and either advances or issues a new command.
					settled = true;
					await refresh();
					if (checkFailed) settled = false;
				} else if (!response.ok) {
					checkFailed = true;
				} else {
					checkFailed = false;
				}
			} catch {
				if (!request.signal.aborted && active()) checkFailed = true;
			} finally {
				if (controller === request) controller = undefined;
				schedule();
			}
		}
		const visibilityChanged = () => {
			if (document.visibilityState === 'visible' && !controller) {
				clearTimeout(timer);
				if (!settled) void checkAttempt();
			}
		};
		const observer = new MutationObserver(() => {
			if (!active()) {
				clearTimeout(timer);
				controller?.abort();
			} else schedule();
		});
		if (dialog) observer.observe(dialog, { attributes: true, attributeFilter: ['open'] });
		document.addEventListener('visibilitychange', visibilityChanged);
		schedule();
		return () => {
			stopped = true;
			clearTimeout(timer);
			controller?.abort();
			observer.disconnect();
			document.removeEventListener('visibilitychange', visibilityChanged);
		};
	});

	async function refresh() {
		if (checking || loading || !onComplete) return;
		checking = true;
		checkFailed = false;
		try {
			await onComplete();
		} catch {
			checkFailed = true;
		} finally {
			checking = false;
		}
	}
</script>

<div bind:this={root} class="flex flex-col gap-3 text-sm w-full min-w-0">
	<p>
		{m.mcps_login_instructions()}
	</p>
	<a
		class="link text-xs"
		href="https://docs.obot.ai/installation/cli-setup"
		target="_blank"
		rel="noopener noreferrer">{m.mcps_login_install()}</a
	>
	<CopyField {id} value={command} label={m.mcps_login_command()} variant="code" />
	<p class="text-muted-content text-xs">
		{m.mcps_login_retry_instructions()}
	</p>
	{#if onComplete}
		{#if checkFailed}
			<p role="alert" class="text-error text-sm">{m.mcps_login_check_failed()}</p>
		{/if}
		<div class="mt-2 flex items-center justify-end gap-3">
			<span role="status" class="flex items-center gap-2 text-sm">
				<Loading class="size-4 shrink-0" />
				{loading || checking ? m.mcps_login_validating() : m.mcps_login_waiting()}
			</span>
			<button
				type="button"
				class="btn btn-ghost btn-sm shrink-0"
				aria-label={m.mcps_login_refresh()}
				title={m.mcps_login_refresh()}
				disabled={loading || checking}
				onclick={refresh}
			>
				<RefreshCw class="size-4" />
			</button>
		</div>
	{/if}
</div>
