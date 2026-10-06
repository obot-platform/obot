<script lang="ts">
	import { m } from '$lib/i18n';
	import type { LaunchType } from '$lib/services';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import { Container, Users } from '@lucide/svelte';

	interface Props {
		onSelectServerType: (type: LaunchType) => void;
		entity?: 'catalog' | 'workspace';
		hideComposite?: boolean;
	}

	let selectServerTypeDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let { onSelectServerType }: Props = $props();

	export function open() {
		selectServerTypeDialog?.open();
	}

	export function close() {
		selectServerTypeDialog?.close();
	}
</script>

<ResponsiveDialog
	title={m.mcps_catalog_select_server_type_title()}
	class="md:w-lg"
	bind:this={selectServerTypeDialog}
>
	<div class="flex flex-col gap-4 p-4 md:p-0">
		<button
			id="add-hosted-server-button"
			class="dark:bg-base-300 hover:bg-base-200 dark:hover:bg-base-400 dark:border-base-400 border-base-300 group bg-base-100 flex cursor-pointer items-center gap-4 rounded-md border px-2 py-4 text-left transition-colors duration-300"
			onclick={() => onSelectServerType('hosted')}
		>
			<Users
				class="text-muted-content size-12 shrink-0 pl-1 transition-colors group-hover:text-inherit"
			/>
			<div>
				<p class="mb-1 text-sm font-semibold">{m.mcps_catalog_select_server_type_hosted()}</p>
				<span class="text-muted-content block text-xs leading-4">
					{m.mcps_catalog_select_server_type_hosted_description()}
				</span>
			</div>
		</button>
		<button
			id="add-remote-server-button"
			class="dark:bg-base-300 hover:bg-base-200 dark:hover:bg-base-400 dark:border-base-400 border-base-300 group bg-base-100 flex cursor-pointer items-center gap-4 rounded-md border px-2 py-4 text-left transition-colors duration-300"
			onclick={() => onSelectServerType('remote')}
		>
			<Container
				class="text-muted-content size-12 shrink-0 pl-1 transition-colors group-hover:text-inherit"
			/>
			<div>
				<p class="mb-1 text-sm font-semibold">{m.mcps_catalog_select_server_type_remote()}</p>
				<span class="text-muted-content block text-xs leading-4">
					{m.mcps_catalog_select_server_type_remote_description()}
				</span>
			</div>
		</button>
	</div>
</ResponsiveDialog>
