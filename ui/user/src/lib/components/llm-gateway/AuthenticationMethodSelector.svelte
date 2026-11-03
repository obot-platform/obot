<script lang="ts">
	import { m } from '$lib/i18n';
	import type { AuthenticationMethod } from '$lib/services/llm-gateway/providers';

	let { methods, value = $bindable('') }: { methods: AuthenticationMethod[]; value?: string } =
		$props();
	const uid = $props.id();
</script>

<fieldset class="flex flex-col gap-2">
	<legend class="mb-2 text-sm font-medium">{m.models_providers_authentication_method()}</legend>
	<div class="tabs tabs-box w-fit max-w-full flex-wrap">
		{#each methods as method (method.id)}
			<label
				class="tab relative has-focus-visible:outline-primary has-focus-visible:outline-2"
				class:tab-active={value === method.id}
			>
				<input
					type="radio"
					class="absolute inset-0 cursor-pointer appearance-none opacity-0"
					name={uid}
					value={method.id}
					bind:group={value}
				/>
				{method.label}
			</label>
		{/each}
	</div>
	<p class="text-muted-content text-xs">
		{methods.find((method) => method.id === value)?.description}
	</p>
</fieldset>
