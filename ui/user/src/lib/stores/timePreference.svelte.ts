import { browser } from '$app/environment';

const store = $state({
	timeFormat: getTimeFormat(),
	setTimeFormat,
	initialize
});

function initialize() {
	if (!browser) {
		return;
	}
	store.timeFormat = getTimeFormat();
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

export default store;
