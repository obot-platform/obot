<script lang="ts">
	import { m } from '$lib/i18n';
	import type { ObotHeader } from '$lib/services';

	interface Props {
		selected?: ObotHeader[];
		readonly?: boolean;
	}

	let { selected = $bindable([]), readonly = false }: Props = $props();
	const uid = $props.id();
	const options = $derived([
		{ header: 'X-Obot-Api-Key-Name', description: m.mcps_inject_headers_api_key_name() },
		{ header: 'X-Obot-Api-Key-Id', description: m.mcps_inject_headers_api_key_id() },
		{ header: 'X-Obot-User-Id', description: m.mcps_inject_headers_user_id() },
		{ header: 'X-Obot-User-Name', description: m.mcps_inject_headers_user_name() },
		{ header: 'X-Obot-User-Email', description: m.mcps_inject_headers_user_email() },
		{ header: 'X-Obot-IDP-User-Id', description: m.mcps_inject_headers_idp_user_id() },
		{ header: 'X-Obot-IDP-Groups', description: m.mcps_inject_headers_idp_groups() }
	] satisfies { header: ObotHeader; description: string }[]);
</script>

<details class="paper">
	<summary class="cursor-pointer text-sm font-semibold">{m.mcps_inject_headers_title()}</summary>
	<div class="mt-4 flex flex-col gap-4">
		<p class="text-sm text-muted-content">{m.mcps_inject_headers_description()}</p>
		{#each options as option (option.header)}
			<div class="flex items-start gap-3">
				<input
					id={`${uid}-${option.header}`}
					type="checkbox"
					class="checkbox checkbox-sm mt-0.5"
					value={option.header}
					bind:group={selected}
					disabled={readonly}
					aria-describedby={`${uid}-${option.header}-description`}
				/>
				<div>
					<label for={`${uid}-${option.header}`} class="font-mono text-sm">{option.header}</label>
					<p id={`${uid}-${option.header}-description`} class="text-xs text-muted-content">
						{option.description}
					</p>
				</div>
			</div>
		{/each}
	</div>
</details>
