import { browser } from '$app/environment';
import { AiClient } from '$lib/constants';

const store = $state({
	timeFormat: getTimeFormat(),
	setTimeFormat,
	initialize,
	aiClientPreference: getAiClientPreference(),
	setAiClientPreference
});

function initialize() {
	if (!browser) {
		return;
	}
	store.timeFormat = getTimeFormat();
	store.aiClientPreference = getAiClientPreference();
}

function setTimeFormat(timeFormat: '12h' | '24h') {
	if (browser) {
		localStorage.setItem('timeFormat', timeFormat);
	}
	store.timeFormat = timeFormat;
}

function getTimeFormat(): '12h' | '24h' {
	if (!browser) {
		return '12h';
	}
	const timeFormat = localStorage.getItem('timeFormat') ?? '12h';
	if (timeFormat === '24h') {
		return '24h';
	}
	return '12h';
}

function setAiClientPreference(aiClient: AiClient | AiClient[]) {
	if (browser) {
		localStorage.setItem(
			'aiClientPreference',
			Array.isArray(aiClient) ? aiClient.join(',') : aiClient
		);
	}
	store.aiClientPreference = Array.isArray(aiClient) ? aiClient : [aiClient];
}

function getAiClientPreference(): AiClient[] | undefined {
	if (!browser) {
		return undefined;
	}
	const aiClientPreference = localStorage.getItem('aiClientPreference')?.split(',');
	return (
		(aiClientPreference as AiClient[]) ?? [
			AiClient.Cursor,
			AiClient.Claude,
			AiClient.Codex,
			AiClient.VSCode
		]
	);
}

export default store;
