<script lang="ts">
	import { m } from '$lib/i18n';
	import { OBOT_HEADERS, type ObotHeader } from '$lib/services';

	interface Props {
		selected?: ObotHeader[];
		readonly?: boolean;
	}

	let { selected = $bindable([]), readonly = false }: Props = $props();
	const uid = $props.id();
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
					checked={selected.some((name) => name.toLowerCase() === option.header.toLowerCase())}
					onchange={(event) => setSelected(option.header, event.currentTarget.checked)}
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
