import { baseURL } from '../http';
import { buildMessagesFromProgress } from './messages';
import * as Operations from './operations';
import * as MessageSource from './thread.svelte';

export default {
	progressToMessages: buildMessagesFromProgress,
	baseURL,
	...Operations,
	...MessageSource
};
