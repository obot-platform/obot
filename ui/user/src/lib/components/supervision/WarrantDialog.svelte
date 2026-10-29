<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { issueWarrant, type Warrant } from '$lib/services/provenance';
	import { success } from '$lib/stores/success';

	// A warrant binds a session to a declared task and the capability scopes it needs.

	interface Props {
		sessionId: string;
		onIssued?: (warrant: Warrant) => void;
	}

	let { sessionId, onIssued }: Props = $props();

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let task = $state('');
	let scopesText = $state('');
	let ttlMinutes = $state<number>(60);
	let saving = $state(false);
	let error = $state('');

	let scopes = $derived(
		scopesText
			.split('\n')
			.map((s) => s.trim())
			.filter(Boolean)
	);
	let valid = $derived(task.trim() !== '' && scopes.length > 0);

	export function open(current?: Warrant) {
		task = current?.task ?? '';
		scopesText = current?.scopes?.join('\n') ?? '';
		ttlMinutes = 60;
		error = '';
		dialog?.open();
	}

	async function submit() {
		if (!valid || saving) return;
		saving = true;
		error = '';
		try {
			const warrant = await issueWarrant(
				sessionId,
				{
					task: task.trim(),
					scopes,
					ttlSeconds: ttlMinutes > 0 ? Math.round(ttlMinutes * 60) : undefined
				},
				{ dontLogErrors: true }
			);
			success.add(m.supervision_session_warrant_issued());
			onIssued?.(warrant);
			dialog?.close();
		} catch (e) {
			error = parseErrorContent(e).message;
		} finally {
			saving = false;
		}
	}
</script>

<ResponsiveDialog
	bind:this={dialog}
	title={m.supervision_session_issue_warrant()}
	class="md:max-w-lg"
>
	<div class="flex flex-col gap-4">
		<p class="text-muted-content text-sm">{m.supervision_session_warrant_description()}</p>

		<div class="flex flex-col gap-1">
			<label for="warrant-task" class="text-sm font-light"
				>{m.supervision_session_warrant_task()}</label
			>
			<input
				id="warrant-task"
				class="text-input-filled"
				placeholder={m.supervision_session_warrant_task_placeholder()}
				bind:value={task}
			/>
		</div>

		<div class="flex flex-col gap-1">
			<label for="warrant-scopes" class="text-sm font-light"
				>{m.supervision_session_warrant_scopes()}</label
			>
			<textarea
				id="warrant-scopes"
				class="text-input-filled min-h-24 font-mono text-sm"
				placeholder={m.supervision_session_warrant_scopes_placeholder()}
				bind:value={scopesText}></textarea>
		</div>

		<div class="flex flex-col gap-1">
			<label for="warrant-ttl" class="text-sm font-light"
				>{m.supervision_session_warrant_ttl()}</label
			>
			<input
				id="warrant-ttl"
				type="number"
				min="1"
				class="text-input-filled"
				bind:value={ttlMinutes}
			/>
		</div>

		{#if error}
			<div class="notification-error rounded-md p-3 text-sm" role="alert">{error}</div>
		{/if}

		<div class="flex justify-end gap-2">
			<button type="button" class="btn btn-secondary text-sm" onclick={() => dialog?.close()}>
				{m.common_cancel()}
			</button>
			<button
				type="button"
				class="btn btn-primary text-sm"
				disabled={!valid || saving}
				onclick={submit}
			>
				{#if saving}<Loading class="size-4" />{:else}{m.supervision_session_issue_warrant()}{/if}
			</button>
		</div>
	</div>
</ResponsiveDialog>
