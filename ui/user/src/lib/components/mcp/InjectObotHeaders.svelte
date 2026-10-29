<script lang="ts">
	import { m } from '$lib/i18n';
	import { OBOT_HEADERS, type ObotHeader } from '$lib/services';
	import Toggle from '../Toggle.svelte';
	import { untrack } from 'svelte';
	import { slide } from 'svelte/transition';

	interface Props {
		selected?: ObotHeader[];
		readonly?: boolean;
	}

	let { selected = $bindable([]), readonly = false }: Props = $props();
	const uid = $props.id();
	let enabled = $state(untrack(() => selected.length > 0));

	function setEnabled(checked: boolean) {
		enabled = checked;
		if (!checked) selected = [];
	}
	const options = $derived([
		{ header: OBOT_HEADERS.apiKeyName, description: m.mcps_inject_headers_api_key_name() },
		{ header: OBOT_HEADERS.apiKeyId, description: m.mcps_inject_headers_api_key_id() },
		{ header: OBOT_HEADERS.userId, description: m.mcps_inject_headers_user_id() },
		{ header: OBOT_HEADERS.userName, description: m.mcps_inject_headers_user_name() },
		{ header: OBOT_HEADERS.userEmail, description: m.mcps_inject_headers_user_email() },
		{ header: OBOT_HEADERS.idpUserId, description: m.mcps_inject_headers_idp_user_id() },
		{ header: OBOT_HEADERS.idpGroups, description: m.mcps_inject_headers_idp_groups() }
	] satisfies { header: ObotHeader; description: string }[]);

	function setSelected(header: ObotHeader, checked: boolean) {
		selected = options
			.map(({ header }) => header)
			.filter((name) =>
				name === header
					? checked
					: selected.some((value) => value.toLowerCase() === name.toLowerCase())
			);
	}
</script>

<div
	class="dark:bg-base-200 dark:border-base-400 bg-base-100 flex flex-col gap-4 rounded-lg border border-transparent p-4 shadow-sm"
>
	<div class="flex justify-between gap-4">
		<button
			type="button"
			class="flex grow cursor-pointer flex-col gap-1 text-left"
			disabled={readonly}
			aria-expanded={enabled}
			aria-controls={`${uid}-headers`}
			onclick={() => setEnabled(!enabled)}
		>
			<h4 class="text-sm font-semibold" class:opacity-50={!enabled}>
				{m.mcps_inject_headers_title()}
			</h4>
			<p class="text-muted-content text-xs font-light">{m.mcps_inject_headers_description()}</p>
		</button>
		<div class="flex self-start">
			<Toggle
				classes={{ label: 'text-sm text-inherit' }}
				label={enabled ? m.mcps_inject_headers_disable() : m.mcps_inject_headers_enable()}
				checked={enabled}
				disabled={readonly}
				onChange={setEnabled}
			/>
		</div>
	</div>
	{#if enabled}
		<div id={`${uid}-headers`} class="flex flex-col gap-4" in:slide={{ axis: 'y' }}>
			{#each options as option (option.header)}
				<div
					class="flex items-start justify-between gap-4"
					role="group"
					aria-label={option.header}
					aria-describedby={`${uid}-${option.header}-description`}
				>
					<div class="flex flex-col gap-1">
						<p class="font-mono text-sm">{option.header}</p>
						<p
							id={`${uid}-${option.header}-description`}
							class="text-muted-content text-xs font-light"
						>
							{option.description}
						</p>
					</div>
					<div class="flex self-start">
						<Toggle
							label={option.header}
							checked={selected.some((name) => name.toLowerCase() === option.header.toLowerCase())}
							disabled={readonly}
							onChange={(checked) => setSelected(option.header, checked)}
						/>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>
