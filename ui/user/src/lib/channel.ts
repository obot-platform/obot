export class Channel<T> {
	readonly #buffer: T[] = [];
	readonly #waiting: ((value: T) => void)[] = [];

	constructor() {
		this.#buffer = [];
		this.#waiting = [];
	}

	send(value: T) {
		if (this.#waiting.length) {
			this.#waiting.shift()?.(value);
		} else {
			this.#buffer.push(value);
		}
	}

	async receive(): Promise<T> {
		if (this.#buffer.length) {
			return this.#buffer.shift()!;
		}
		return new Promise((resolve) => {
			if (this.#buffer.length) {
				resolve(this.#buffer.shift()!);
			} else {
				this.#waiting.push(resolve);
			}
		});
	}
}
