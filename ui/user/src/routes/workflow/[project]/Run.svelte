<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import { toHTMLFromMarkdown } from '$lib/markdown';
	import { formatTime } from '$lib/time';
	import { Copy, Edit, MessageCircleMore } from 'lucide-svelte';
	import { fade, slide } from 'svelte/transition';
	import ChatInput from '$lib/components/messages/Input.svelte';

	interface Props {
		name: string;
	}

	let { name }: Props = $props();

	const taskArguments: {
		name: string;
		value: string;
	}[] = $state([
		{
			name: 'CompanyName',
			value: 'Obot'
		}
	]);

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
					content: `## 1) Salesforce – Account, Contacts, Opportunities, Documents

### Account: Obot

* **Account Name:** Obot
* **Account Id:** \`0015g00000ABC123AA\`
* **Account Owner:** Alex Johnson
* **Industry:** Software
* **Billing Country:** United States
* **Website:** \`https://www.obot.ai\`

---

### Related Contacts (with roles & emails)

| Contact Name   | Title               | Role              | Email                                                   | Phone             | Primary? |
| -------------- | ------------------- | ----------------- | ------------------------------------------------------- | ----------------- | -------- |
| Jane Doe       | VP of Operations    | Decision Maker    | [jane.doe@obot.ai](mailto:jane.doe@obot.ai)             | +1 (555) 201-1001 | Yes      |
| Michael Lee    | Director of Finance | Billing Contact   | [michael.lee@obot.ai](mailto:michael.lee@obot.ai)       | +1 (555) 201-1002 |          |
| Priya Sharma   | Head of Security    | Technical Contact | [priya.sharma@obot.ai](mailto:priya.sharma@obot.ai)     | +1 (555) 201-1003 |          |
| Chris Martinez | Operations Manager  | Day-to-day Admin  | [chris.martinez@obot.ai](mailto:chris.martinez@obot.ai) | +1 (555) 201-1004 |          |

*(Roles are pulled from Contact Role / custom role fields where available; otherwise inferred from title.)*

---

### Opportunities – finding the **most recent Closed Won** membership

Relevant opportunities for Account \`0015g00000ABC123AA\`:

1. **“2024 Membership – Obot”**

   * Opportunity Id: \`0065g00000MNO456AA\`
   * Stage: Closed Won
   * **Close Date:** 2024-03-10
   * Amount: $18,000
   * Membership Level (custom field): **Standard**

2. **“2025 Membership – Obot – Upgrade”** ✅ *Most recent Closed Won*

   * Opportunity Id: \`0065g00000XYZ789AA\`
   * Stage: **Closed Won**
   * **Close Date:** **2025-03-15**
   * Amount: $32,000
   * Membership Level (custom field): **Premier**
   * Term: 12 months
   * Primary Contact: Jane Doe
   * Notes (Opportunity):

     > Upgraded from Standard to Premier – includes priority support and dedicated CSM.

For step 3, we use:

* **Membership Level:** \`Premier\`
* **Join Date (per instructions):** **2025-03-15** (the Closed Won date from this opportunity)

---

### Documents for Obot

Using the Account Id \`0015g00000ABC123AA\`, I looked for:

#### Classic Attachments (Attachment object – \`ParentId = Account.Id\`)

Mocked results:

1. **Name:** \`Obot - Signed Membership Agreement 2025.pdf\`

   * Attachment Id: \`00P5g00000ATT001AA\`
   * ParentId: \`0015g00000ABC123AA\`
   * Description: Signed Premier membership agreement.

2. **Name:** \`Obot - Data Processing Addendum.pdf\`

   * Attachment Id: \`00P5g00000ATT002AA\`
   * ParentId: \`0015g00000ABC123AA\`
   * Description: Custom DPA, referenced in Account Description.

#### Salesforce Files (ContentDocument via ContentDocumentLink)

Found ContentDocumentLink records where:

* \`LinkedEntityId = 0015g00000ABC123AA\`

Mocked files:

1. **ContentDocument:** \`0695g00000DOC001AA\`

   * Title: \`Obot Onboarding Plan\`
   * File Type: Google Doc / PDF
   * Description: Internal onboarding checklist for CSM.

2. **ContentDocument:** \`0695g00000DOC002AA\`

   * Title: \`Obot - Logo Pack\`
   * File Type: PNG/ZIP
   * Description: Logo assets used for marketing and listings.

3. **ContentDocument:** \`0695g00000DOC003AA\`

   * Title: \`Obot Implementation Notes\`
   * File Type: \`Doc\`
   * Description: Internal notes from discovery and technical calls.

These documents would be referenced or linked in the Google Sheet row (often as URLs or Salesforce record links).

---

## 2) Demo Workflow LF Google Sheet – structure (mocked)

After opening **“Demo Workflow LF”** in Google Sheets, I skimmed the header row and first few example rows.

Mocked columns (based on what I “saw” in the sheet):

1. \`Account Name\`
2. \`Membership Level\`
3. \`Join Date\`
4. \`Status\`
5. \`Primary Contact Name\`
6. \`Primary Contact Email\`
7. \`Billing Contact\`
8. \`Billing Email\`
9. \`Salesforce Account Id\`
10. \`Primary Opportunity Id\`
11. \`CSM Owner\`
12. \`Region\`
13. \`Docs / Links\`
14. \`Internal Notes\`

Example existing row for context (mocked):

| Account Name | Membership Level | Join Date  | Status | Primary Contact Name | Primary Contact Email                               | Billing Contact | Billing Email                               | Salesforce Account Id | Primary Opportunity Id | CSM Owner   | Region | Docs / Links       | Internal Notes                     |
| ------------ | ---------------- | ---------- | ------ | -------------------- | --------------------------------------------------- | --------------- | ------------------------------------------- | --------------------- | ---------------------- | ----------- | ------ | ------------------ | ---------------------------------- |
| Acme Corp    | Standard         | 2024-01-05 | Active | Sarah Green          | [sarah.green@acme.com](mailto:sarah.green@acme.com) | John Brown      | [billing@acme.com](mailto:billing@acme.com) | 0015g00000AAA111AA    | 0065g00000AAA111AA     | Emily Davis | NA     | SF Docs, Logo, DPA | Onboarding complete; QBR in March. |

I then followed this exact style and formatting for Obot.

---

## 3) New row for **Obot** in Demo Workflow LF Sheet

Using the Salesforce info above and matching the sheet’s style, I would add the following row (mocked values):

| Account Name | Membership Level | Join Date  | Status | Primary Contact Name | Primary Contact Email                       | Billing Contact | Billing Email                                     | Salesforce Account Id | Primary Opportunity Id | CSM Owner    | Region | Docs / Links                                                                                                                                 | Internal Notes                                                                                                            |
| ------------ | ---------------- | ---------- | ------ | -------------------- | ------------------------------------------- | --------------- | ------------------------------------------------- | --------------------- | ---------------------- | ------------ | ------ | -------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------- |
| Obot         | Premier          | 2025-03-15 | Active | Jane Doe             | [jane.doe@obot.ai](mailto:jane.doe@obot.ai) | Michael Lee     | [michael.lee@obot.ai](mailto:michael.lee@obot.ai) | 0015g00000ABC123AA    | 0065g00000XYZ789AA     | Alex Johnson | NA     | Membership Agreement, DPA, Onboarding Plan, Implementation Notes, Logo Pack (Salesforce Attachments/Files linked via Account & Content Docs) | Prefers **quarterly billing**. Include security lead on all onboarding calls. Upgraded from Standard to Premier for 2025. |

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
	<h2 class="p-4 pr-12 text-xl font-semibold">
		{name} | {formatTime(new Date(Date.now() - 1000 * 60 * 60 * 24).toISOString())}
	</h2>
	<div class="default-scrollbar-thin relative h-[calc(100%-48px)] w-full overflow-y-auto">
		<div class="mb-4 w-full px-4">
			{#if taskArguments.length > 0}
				{#each taskArguments as argument (argument.name)}
					<div
						class="bg-primary/15 flex w-fit flex-wrap items-center gap-2 rounded-full px-2 py-1 text-sm"
					>
						<p class="text-primary font-medium">
							${argument.name}:
						</p>
						<p>{argument.value}</p>
					</div>
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
		{#if showChat}
			<div
				class="bg-background dark:bg-surface2 sticky bottom-0 left-0 w-full p-4 pb-8"
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
		{:else}
			<div class="sticky bottom-0 left-0 w-full p-4 pb-8" in:slide={{ axis: 'y' }}>
				<button
					class="button-icon bg-primary mx-auto text-white transition-all hover:scale-110"
					onclick={() => (showChat = !showChat)}
				>
					<MessageCircleMore class="size-6" />
				</button>
			</div>
		{/if}
	</div>
</div>

<style lang="postcss">
	:global {
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
