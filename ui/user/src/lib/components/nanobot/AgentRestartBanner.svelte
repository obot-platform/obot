<!-- AgentRestartBanner removed as unused component. -->

{#if showBanner}
	<div
		class="border-warning/40 bg-warning/10 text-warning-content flex items-start gap-3 rounded-xl border px-4 py-3"
	>
		<AlertTriangle class="text-warning mt-0.5 size-5 shrink-0" />
		<div class="min-w-0 flex-1">
			<p class="text-sm font-medium">Agent restart recommended</p>
			<p class="text-sm/5 opacity-80">{bannerMessage}</p>
		</div>
		<div class="flex shrink-0 items-center gap-2">
			<button
				type="button"
				class="btn btn-warning btn-sm"
				onclick={() => {
					showRestartAgentConfirm = true;
				}}
			>
				{#if restartingAgent || loadingAgentServer}
					<LoaderCircle class="size-4 animate-spin" />
				{/if}
				Restart agent
			</button>
			<button
				type="button"
				class="btn btn-ghost btn-sm"
				onclick={() => {
					dismissed = true;
				}}
			>
				Dismiss
			</button>
		</div>
	</div>
{/if}

<Confirm
	show={showRestartAgentConfirm}
	title="Restart Agent"
	msg="Restart this agent?"
	note="This will temporarily interrupt any active agent sessions."
	onsuccess={handleRestartAgent}
	oncancel={() => {
		showRestartAgentConfirm = false;
	}}
	loading={restartingAgent}
	type="info"
/>
