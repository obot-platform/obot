import type { Action } from 'svelte/action';

/**
 * Modal dialogs stay above popovers, so a visible toast is moved into the topmost
 * modal. Listening starts only while a toast is visible: a `toggle` listener for
 * dialog open/close, plus direct-child observers on that dialog's ancestors in
 * case the dialog is removed. Nothing is observed while the stack is empty.
 */
const hosts: HTMLElement[] = [];
const openHosts = new Set<HTMLElement>();
const homes = new WeakMap<HTMLElement, ParentNode>();
const modalStack: HTMLDialogElement[] = [];
const dialogParentObservers = new Map<HTMLDialogElement, MutationObserver>();
let stopToggle: (() => void) | undefined;

function topModal() {
	for (let i = modalStack.length - 1; i >= 0; i--) {
		const dialog = modalStack[i];
		if (dialog.isConnected && dialog.open && dialog.matches(':modal')) return dialog;
		modalStack.splice(i, 1);
	}
}

function noteDialog(dialog: HTMLDialogElement) {
	const index = modalStack.indexOf(dialog);
	if (index >= 0) modalStack.splice(index, 1);
	if (dialog.isConnected && dialog.open && dialog.matches(':modal')) modalStack.push(dialog);
}

function maybeUnwatch(dialog: HTMLDialogElement) {
	const hostInside = hosts.some((host) => host.parentElement === dialog);
	if (hostInside && dialog.isConnected) return;
	dialogParentObservers.get(dialog)?.disconnect();
	dialogParentObservers.delete(dialog);
}

function watchParent(dialog: HTMLDialogElement) {
	if (dialogParentObservers.has(dialog) || !dialog.parentNode) return;
	const observer = new MutationObserver(() => {
		if (dialog.isConnected) return;
		maybeUnwatch(dialog);
		noteDialog(dialog);
		if (rescueDetached()) place();
	});

	let node: Node | null = dialog;
	while (node?.parentNode) {
		observer.observe(node.parentNode, { childList: true });
		node = node.parentNode;
	}
	dialogParentObservers.set(dialog, observer);
}

function pinInDialog(host: HTMLElement, modal: HTMLDialogElement) {
	const previous = host.parentElement;
	if (host.matches(':popover-open')) host.hidePopover();
	// Inline display wins over the :not(:popover-open) { display: none } rule.
	host.style.display = 'flex';
	host.style.zIndex = '100000';
	if (previous !== modal) modal.append(host);
	if (previous instanceof HTMLDialogElement && previous !== modal) maybeUnwatch(previous);
	watchParent(modal);
}

function release(host: HTMLElement, show: boolean) {
	const previous = host.parentElement;
	host.style.removeProperty('display');
	host.style.removeProperty('z-index');
	const home = homes.get(host);
	if (home?.isConnected && host.parentElement !== home) home.append(host);
	if (previous instanceof HTMLDialogElement) maybeUnwatch(previous);
	if (!show) {
		if (host.isConnected && host.matches(':popover-open')) host.hidePopover();
		return;
	}
	if (host.isConnected && !host.matches(':popover-open')) host.showPopover();
}

function place() {
	const modal = topModal();
	for (const host of hosts) {
		const show = openHosts.has(host);
		if (show && modal) pinInDialog(host, modal);
		else release(host, show);
	}
}

function rescueDetached() {
	let rescued = false;
	for (const host of hosts) {
		if (host.isConnected) continue;
		const home = homes.get(host);
		if (!home?.isConnected) continue;
		home.append(host);
		rescued = true;
	}
	return rescued;
}

function startListening() {
	for (const dialog of document.querySelectorAll('dialog[open]')) {
		if (dialog instanceof HTMLDialogElement) noteDialog(dialog);
	}
	const onToggle = (event: Event) => {
		if (!(event.target instanceof HTMLDialogElement)) return;
		noteDialog(event.target);
		place();
	};
	document.addEventListener('toggle', onToggle, true);
	stopToggle = () => document.removeEventListener('toggle', onToggle, true);
}

function stopListening() {
	stopToggle?.();
	stopToggle = undefined;
	for (const observer of dialogParentObservers.values()) observer.disconnect();
	dialogParentObservers.clear();
	modalStack.length = 0;
}

function syncListening() {
	if (openHosts.size > 0) {
		if (!stopToggle) startListening();
		return;
	}
	if (stopToggle || dialogParentObservers.size > 0) stopListening();
}

function setOpen(node: HTMLElement, open: boolean) {
	if (open) openHosts.add(node);
	else openHosts.delete(node);
	syncListening();
	place();
}

export const notificationPopover: Action<HTMLElement, boolean> = (node, open) => {
	const home = node.parentElement;
	if (home) homes.set(node, home);
	hosts.push(node);
	setOpen(node, open);

	return {
		update(next) {
			setOpen(node, next);
		},
		destroy() {
			const index = hosts.indexOf(node);
			if (index >= 0) hosts.splice(index, 1);
			openHosts.delete(node);
			release(node, false);
			syncListening();
		}
	};
};
