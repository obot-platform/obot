<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import { toHTMLFromMarkdown } from '$lib/markdown';
	import { formatTime } from '$lib/time';
	import { Copy, Edit, MessageCircleMore, MessageCircleOff } from 'lucide-svelte';
	import { fade, slide } from 'svelte/transition';
	import ChatInput from '$lib/components/messages/Input.svelte';

	interface Props {
		name: string;
		args: {
			name: string;
			displayLabel: string;
		}[];
		values: Record<string, string>;
		run?: {
			id: string;
			created: string;
		};
	}

	let { name, args, values, run }: Props = $props();

	let showChat = $state(false);

	const tasks: {
		id: string;
		name: string;
		loading?: boolean;
		messages: {
			id: string;
			type: string;
			name: string;
			tool?: string;
			content?: string;
			created: string;
		}[];
	}[] = $state([
		{
			id: '1',
			name: 'Add Member to Google Sheet',
			loading: false,
			arguments: [
				{
					name: 'CompanyName',
					value: 'Obot'
				}
			],
			messages: [
				{
					id: '1.1',
					type: 'tool-call',
					name: 'Salesforce',
					tool: 'get_record',
					content: '',
					created: new Date(new Date().getTime() - 1000 * 60 * 60 * 24).toISOString()
				},
				{
					id: '1.2',
					type: 'tool-call',
					name: 'Google Sheets',
					tool: 'read_spreadsheet',
					content: '',
					// 2 minutes after previous message
					created: new Date(new Date().getTime() - 1000 * 60 * 2).toISOString()
				},
				{
					id: '1.3',
					type: 'tool-call',
					name: 'Google Sheets',
					tool: 'append_row',
					content: '',
					// 3 minutes after previous message
					created: new Date(new Date().getTime() - 1000 * 60 * 3).toISOString()
				},
				{
					id: '1.4',
					type: 'message',
					name: 'Onboarding Workflow',
					// 30 seconds after previous message
					created: new Date(new Date().getTime() - 1000 * 30).toISOString(),
					content: `
* **Account Name:** Obot
* **Account Id:** \`0015g00000ABC123AA\`
* **Account Owner:** Alex Johnson
* **Industry:** Software
* **Billing Country:** United States
* **Website:** \`https://www.obot.ai\`

### New row for **Obot** in Demo Workflow LF Sheet

| Account Name | Membership Level | Join Date  | Status | Primary Contact Name | Primary Contact Email                       | Billing Contact | Billing Email                                     | Salesforce Account Id | Primary Opportunity Id | CSM Owner    | Region |
| ------------ | ---------------- | ---------- | ------ | -------------------- | ------------------------------------------- | --------------- | ------------------------------------------------- | --------------------- | ---------------------- | ------------ | ------ |
| Obot         | Premier          | 2025-03-15 | Active | Jane Doe             | [jane.doe@obot.ai](mailto:jane.doe@obot.ai) | Michael Lee     | [michael.lee@obot.ai](mailto:michael.lee@obot.ai) | 0015g00000ABC123AA    | 0065g00000XYZ789AA     | Alex Johnson | NA     |

Key points tied to your instructions:

* **Join Date** = **2025-03-15**, which is the **Close Date of the most recent Closed Won opportunity** for Obot.
* Documents are captured in the **Docs / Links** column and would typically be added as live hyperlinks to the Files/Attachments or a shared folder.
* Notes and special instructions are carried over from the **Account Description** and Opportunity notes into **Internal Notes**.

---

If you’d like, I can next:

* Adjust the row to match a different column schema if your real sheet uses other headings, or
* Add a few more mocked companies/rows in the same style for testing your workflow.
 `
				}
			]
		},
		{
			id: '2',
			name: 'Add New Member Contacts to Google Groups',
			messages: [],
			loading: true
		},
		{
			id: '3',
			name: 'Add member contacts to Slack',
			messages: [],
			loading: true
		},
		{
			id: '4',
			name: 'Add Logo to site',
			messages: [],
			loading: true
		},
		{
			id: '5',
			name: 'Send Welcome Email',
			messages: [],
			loading: true
		}
	]);
</script>

<div class="h-full w-full">
	<h2
		class="border-l-primary dark:border-b-surface2 border-b border-l-4 border-b-transparent p-4 pr-12 text-xl font-semibold shadow-xs"
	>
		{name}
		{run?.created ? `| ${formatTime(run.created)}` : ''}
	</h2>
	<div class="default-scrollbar-thin relative h-[calc(100%-48px)] w-full overflow-y-auto">
		<div class="mb-4 flex w-full flex-wrap gap-2 px-4 pt-4">
			{#if args.length > 0}
				{#each args as argument (argument.name)}
					{#if values[argument.name]}
						<div
							class="bg-primary/15 flex w-fit flex-wrap items-center gap-2 rounded-full px-2 py-1 text-sm"
						>
							<p class="text-primary font-medium">
								${argument.name}:
							</p>
							<p>{values[argument.name]}</p>
						</div>
					{/if}
				{/each}
			{/if}
		</div>
		<div class="flex flex-col gap-8 px-4">
			{#each tasks as task (task.id)}
				<div class="flex w-full flex-col gap-2">
					<h3 class="flex items-center gap-2 text-xl font-semibold">
						<span class={task.loading ? 'opacity-30' : ''}>{task.name}</span>
						{#if task.loading}
							<Loading class="text-primary size-4" />
						{/if}
					</h3>
					<div class="flex flex-col gap-2 px-4">
						{#each task.messages as message (message.id)}
							<div class="w-full">
								<div class="mb-1 flex items-center space-x-2">
									{#if message.type === 'tool-call'}
										<span class="text-sm font-semibold">{message.name} -> {message.tool}</span>
									{:else}
										<span class="text-sm font-semibold">{message.name}</span>
									{/if}
									{#if message.created}
										<span class="text-gray text-sm">{formatTime(message.created)}</span>
									{/if}

									{#if message.type === 'tool-call'}
										<div class="flex items-center gap-2">
											<button class="text-gray cursor-pointer text-xs underline" onclick={() => {}}>
												Show Details
											</button>
										</div>
									{/if}
								</div>
								{#if message.content}
									<div
										class="milkdown-content workflow-run pt-2"
										transition:fade={{ duration: 1000 }}
									>
										{@html toHTMLFromMarkdown(message.content)}
									</div>
								{/if}
								{#if message.type !== 'tool-call'}
									<div class="flex items-center gap-2">
										<div>
											<button class="icon-button-small">
												<Copy class="size-4" />
											</button>
										</div>

										<div>
											<button
												use:tooltip={'Open message in editor'}
												class="icon-button-small"
												onclick={() => {}}
											>
												<Edit class="size-4" />
											</button>
										</div>
									</div>
								{/if}
							</div>
						{/each}
					</div>
				</div>
			{/each}
		</div>
		<div class="sticky bottom-0 left-0 w-full pb-4">
			<div class="flex w-full justify-end pr-6" class:pb-4={!showChat} class:pb-1={showChat}>
				<button
					class="button-icon bg-primary text-white transition-all hover:scale-110"
					onclick={() => (showChat = !showChat)}
					use:tooltip={'Toggle chat'}
				>
					{#if showChat}
						<MessageCircleOff class="size-6" />
					{:else}
						<MessageCircleMore class="size-6" />
					{/if}
				</button>
			</div>
			{#if showChat}
				<div
					class="workflow-run bg-background dark:bg-surface2 w-full p-4"
					in:slide={{ axis: 'y' }}
				>
					<ChatInput
						classes={{
							root: 'mt-0'
						}}
						onSubmit={async (i) => {
							//	await thread?.invoke(i);
						}}
						placeholder="What can I help with?"
					/>
				</div>
			{/if}
		</div>
	</div>
</div>

<style lang="postcss">
	:global {
		.workflow-run .milkdown {
			background-color: var(--color-surface1);
		}
		.milkdown-content.workflow-run {
			h1 {
				font-size: var(--text-xl);
			}
			h2 {
				font-size: var(--text-lg);
			}
			h3,
			h4 {
				font-size: var(--text-base);
			}
		}
	}
</style>
