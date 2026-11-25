import { goto } from '$lib/url';
import { notifications } from './stores';
import { tick } from 'svelte';

/**
 * Shared form validation and save behavior.
 *
 * Every page-level form follows the same contract:
 * - The Save button is always enabled (only `saving` / `readonly` may disable it).
 * - Clicking Save calls `await validation.check()`. When it fails, every invalid field is
 *   highlighted and the first invalid field in DOM order is scrolled into view and focused.
 * - After a check, only the fields that failed it keep reporting errors. Editing never raises a new
 *   error on another field (for example a "mismatch" while the user is still typing); a field's
 *   error clears as soon as it is fixed and returns only on the next `check()`.
 * - Saving an existing object stays on the page and toasts; saving a new object toasts and
 *   replaces the URL with the object's detail route (`completeSave`).
 * - Cancel navigates to the parent list route (`FORM_PARENT_ROUTES` in `$lib/constants`).
 *
 * Markup convention for a field:
 *
 * ```svelte
 * <label for="x" class:error={v.has('name')}>Name</label>
 * <input class="text-input-filled" class:error={v.has('name')} {...v.attrs('name', 'x')} />
 * <FieldError id="x" message={v.message('name')} />
 * ```
 *
 * For section-level errors (an empty list, an invalid row), put `data-invalid` on the wrapper and
 * style it with `ring-1 ring-error rounded-md`; focus lands on the first focusable control inside.
 */

/** `string` = error message to display, `true` = highlight only. */
export type FormErrors<K extends string> = Partial<Record<K, string | true>>;

export class FormValidation<K extends string> {
	submitted = $state(false);
	#external = $state<FormErrors<K>>({});
	#validate: () => FormErrors<K>;

	/** Keys that failed the last `check()` and have not been fixed since. */
	#flagged = $state<ReadonlySet<K>>(new Set());

	errors = $derived.by<FormErrors<K>>(() => {
		if (!this.submitted) return {};
		const current = this.#validate();
		const shown: FormErrors<K> = {};
		for (const key of this.#flagged) {
			if (current[key] !== undefined) shown[key] = current[key];
		}
		return { ...shown, ...this.#external };
	});

	valid = $derived(Object.keys(this.errors).length === 0);

	root: HTMLElement | undefined;

	constructor(validate: () => FormErrors<K>) {
		this.#validate = validate;
	}

	/** Use with `{@attach validation.attach}` on the form root to scope focus lookup. */
	attach = (node: HTMLElement) => {
		this.root = node;
		node.addEventListener('input', this.#prune);
		node.addEventListener('change', this.#prune);
		return () => {
			node.removeEventListener('input', this.#prune);
			node.removeEventListener('change', this.#prune);
			if (this.root === node) this.root = undefined;
		};
	};

	/** Drops flagged fields that are now valid, so an edit can only clear errors, never restore them. */
	#prune = () => {
		if (!this.submitted || this.#flagged.size === 0) return;
		const current = this.#validate();
		const remaining = [...this.#flagged].filter((key) => current[key] !== undefined);
		if (remaining.length !== this.#flagged.size) this.#flagged = new Set(remaining);
	};

	has(key: K): boolean {
		return this.errors[key] !== undefined;
	}

	message(key: K): string | undefined {
		const error = this.errors[key];
		return typeof error === 'string' ? error : undefined;
	}

	/**
	 * Attributes to spread on the input for `key`. Pass `hintId` when the field has persistent
	 * helper text: it stays in `aria-describedby`, with the error appended while invalid.
	 */
	attrs(key: K, id: string, hintId?: string) {
		const invalid = this.has(key);
		const describedBy = [hintId, invalid && this.message(key) ? `${id}-error` : undefined]
			.filter(Boolean)
			.join(' ');
		return {
			id,
			'aria-invalid': invalid ? ('true' as const) : undefined,
			'aria-describedby': describedBy || undefined,
			'data-invalid': invalid ? '' : undefined
		};
	}

	/** Sets or clears an error that the synchronous validator can't compute (server checks). */
	setExternal(key: K, message?: string | true) {
		const next = { ...this.#external };
		if (message === undefined) delete next[key];
		else next[key] = message;
		this.#external = next;
	}

	clearExternal() {
		this.#external = {};
	}

	/** Shows errors and focuses the first invalid field. Resolves to whether the form is valid. */
	async check(): Promise<boolean> {
		this.submitted = true;
		this.#flagged = new Set(Object.keys(this.#validate()) as K[]);
		if (Object.keys(this.errors).length === 0) return true;
		await tick();
		focusFirstInvalid(this.root ?? document);
		return false;
	}

	reset() {
		this.submitted = false;
		this.#flagged = new Set();
		this.#external = {};
	}
}

const FOCUSABLE =
	'input:not([type="hidden"]):not([disabled]), select:not([disabled]), textarea:not([disabled]), button:not([disabled]), [tabindex]:not([tabindex="-1"]), [contenteditable="true"]';

function isFocusable(el: Element): boolean {
	return el instanceof HTMLElement && el.matches(FOCUSABLE);
}

/** Anything marked invalid: explicit markers, ARIA state, or the shared `.error` input class. */
const INVALID = '[data-invalid], [aria-invalid="true"], input.error, textarea.error, select.error';

/**
 * Scrolls to and focuses the first invalid element under `root`, in DOM order. When that element
 * isn't focusable, focuses its first focusable descendant, or the element itself via
 * `tabindex="-1"`.
 */
export function focusFirstInvalid(root: ParentNode = document) {
	const el = root.querySelector<HTMLElement>(INVALID);
	if (!el) return;

	el.scrollIntoView({ block: 'center', behavior: 'smooth' });

	let target: HTMLElement | null = isFocusable(el) ? el : el.querySelector<HTMLElement>(FOCUSABLE);
	if (!target) {
		el.tabIndex = -1;
		target = el;
	}
	target.focus({ preventScroll: true });
}

/**
 * Finishes a successful save: toasts, and for a newly created object replaces the current URL
 * (for example `?new`) with the object's detail route so Back doesn't reopen the create form.
 */
export async function completeSave(opts: { created: boolean; message: string; href?: string }) {
	notifications.add(opts.message, 'success');
	if (opts.created && opts.href) {
		await goto(opts.href, { replaceState: true });
	}
}
