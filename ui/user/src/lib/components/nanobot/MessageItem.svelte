<script lang="ts">
	import type {
		Attachment,
		ChatResult,
		ChatMessageItem,
		ResourceContents
	} from '$lib/services/nanobot/types';
	import { parseToolFilePath, isPolicyViolation } from '$lib/services/nanobot/utils';
	import MessageItemAudio from './MessageItemAudio.svelte';
	import MessageItemFile from './MessageItemFile.svelte';
	import MessageItemImage from './MessageItemImage.svelte';
	import MessageItemPolicyViolation from './MessageItemPolicyViolation.svelte';
	import MessageItemResource from './MessageItemResource.svelte';
	import MessageItemResourceLink from './MessageItemResourceLink.svelte';
	import MessageItemText from './MessageItemText.svelte';
	import MessageItemTool from './MessageItemTool.svelte';

	interface Props {
		item: ChatMessageItem;
		role: 'user' | 'assistant';
		onSend?: (message: string, attachments?: Attachment[]) => Promise<ChatResult | void>;
		onFileOpen?: (filename: string) => void;
		onReadResource?: (uri: string) => Promise<{ contents: ResourceContents[] }>;
	}

	let { item, role, onSend, onFileOpen, onReadResource }: Props = $props();
</script>

{#if item.type === 'text' && isPolicyViolation(item.text)}
	<MessageItemPolicyViolation {item} {role} />
{:else if item.type === 'text'}
	<MessageItemText {item} {role} />
{:else if item.type === 'image'}
	<MessageItemImage {item} />
{:else if item.type === 'audio'}
	<MessageItemAudio {item} />
{:else if item.type === 'resource_link'}
	<MessageItemResourceLink {item} {onReadResource} />
{:else if item.type === 'resource'}
	<MessageItemResource {item} />
{:else if item.type === 'tool'}
	{@const filePath = item.name === 'write' && item.arguments ? parseToolFilePath(item) : null}
	{@const isWrittenFile = !!filePath && !filePath.includes('/.nanobot/')}
	{#if isWrittenFile}
		<MessageItemFile {item} {onFileOpen} />
	{:else}
		<MessageItemTool {item} {onSend} />
	{/if}
{/if}
