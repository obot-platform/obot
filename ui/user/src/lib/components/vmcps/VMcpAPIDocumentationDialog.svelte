<script lang="ts">
	import CopyButton from '$lib/components/CopyButton.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import { m } from '$lib/i18n';
	import { UserService, type VMCP, type VMCPAPIProxyDocumentation } from '$lib/services';

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let vmcp = $state<VMCP>();
	let documentation = $state<VMCPAPIProxyDocumentation>();
	let loading = $state(false);
	let loadError = $state(false);

	export async function open(value: VMCP) {
		vmcp = value;
		documentation = undefined;
		loadError = false;
		loading = true;
		dialog?.open();
		try {
			documentation = await UserService.getVMCPAPIProxyDocumentation(value.id);
		} catch {
			loadError = true;
		} finally {
			loading = false;
		}
	}
</script>

<ResponsiveDialog
	bind:this={dialog}
	title={m.vmcps_api_docs_title({ name: vmcp?.displayName ?? 'vMCP' })}
	onClose={() => {
		vmcp = undefined;
		documentation = undefined;
	}}
>
	<div class="default-scrollbar-thin max-h-[70vh] overflow-y-auto">
		{#if loading}
			<p class="text-muted-content text-sm">{m.vmcps_loading()}</p>
		{:else if loadError}
			<p class="text-destructive text-sm">{m.vmcps_api_docs_load_failed()}</p>
		{:else if documentation?.components.length}
			<p class="text-muted-content mb-4 text-sm">{m.vmcps_api_docs_description()}</p>
			{#each documentation.components as component (component.id)}
				<section class="mb-6 space-y-3" aria-labelledby={`api-doc-${component.id}`}>
					<div class="flex items-center justify-between gap-3">
						<h3 id={`api-doc-${component.id}`} class="text-sm font-semibold">
							{component.displayName}
						</h3>
						<CopyButton
							text={component.basePath}
							noButtonText
							tooltipText={m.vmcps_api_docs_copy_base()}
						/>
					</div>
					<label class="text-muted-content text-xs font-medium" for={`base-${component.id}`}>
						{m.vmcps_api_docs_base_path()}
					</label>
					<input
						id={`base-${component.id}`}
						readonly
						class="text-input-filled w-full font-mono text-xs"
						value={component.basePath}
					/>
					<div class="space-y-3">
						{#each component.operations as operation (operation.operationKey)}
							<article class="rounded-lg border border-base-300 p-3 dark:border-base-400">
								<div class="flex flex-wrap items-center gap-2">
									<span class="rounded bg-primary/10 px-2 py-0.5 font-mono text-xs text-primary">
										{operation.method}
									</span>
									<strong class="text-sm">{operation.displayName}</strong>
									{#if operation.businessAuthRequired}
										<span class="rounded bg-warning/10 px-2 py-0.5 text-xs text-warning">
											{m.vmcps_api_docs_auth_required()}
										</span>
									{/if}
								</div>
								{#if operation.description}
									<p class="text-muted-content mt-1 text-sm">{operation.description}</p>
								{/if}
								<dl class="mt-2 grid gap-2 text-xs sm:grid-cols-2">
									<div>
										<dt class="font-medium">{m.vmcps_api_docs_path()}</dt>
										<dd class="font-mono break-all">{operation.pathTemplate}</dd>
									</div>
									{#each operation.parameters ?? [] as parameter (parameter.in + parameter.name)}
										<div>
											<dt class="font-medium">
												{parameter.name} ({parameter.in}{parameter.required ? ', required' : ''})
											</dt>
											<dd class="break-all">{JSON.stringify(parameter.schema ?? {})}</dd>
										</div>
									{/each}
								</dl>
							</article>
						{/each}
					</div>
				</section>
			{/each}
		{:else}
			<p class="text-muted-content text-sm">{m.vmcps_api_docs_empty()}</p>
		{/if}
	</div>
</ResponsiveDialog>
