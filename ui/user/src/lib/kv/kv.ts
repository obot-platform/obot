import { getContext, setContext } from 'svelte';

const CONTEXT_KEY = '@obot/context/kv';

/**
 * Production-ready IndexedDB Key-Value Store with expiration support
 * Features: TTL,  cleanup, and error handling
 */
export class KV {
	dbName: string;
	storeName: string;
	version: number;
	db: IDBDatabase | null;
	cleanupInterval: number | null;
	initPromise: Promise<IDBDatabase>;

	constructor(dbName = 'obot', storeName = 'main', version = 1) {
		this.dbName = dbName;
		this.storeName = storeName;
		this.version = version;
		this.db = null;
		this.cleanupInterval = null;
		this.initPromise = this.init();
	}

	/**
	 * Initialize database connection
	 */
	async init() {
		if (this.db) return this.db;

		return new Promise<IDBDatabase>((resolve, reject) => {
			const request = indexedDB.open(this.dbName, this.version);

			request.onerror = () => reject(new Error(`Failed to open database: ${request.error}`));

			request.onsuccess = () => {
				this.db = request.result;
				this.startCleanupTask();
				resolve(this.db);
			};

			request.onupgradeneeded = (event) => {
				const db = (event?.target as IDBOpenDBRequest)?.result;

				// Create object store if it doesn't exist
				if (!db.objectStoreNames.contains(this.storeName)) {
					const objectStore = db.createObjectStore(this.storeName, { keyPath: 'key' });

					// Create indexes for efficient queries
					objectStore.createIndex('expiry', 'expiry', { unique: false });
					objectStore.createIndex('createdAt', 'createdAt', { unique: false });
				}
			};

			request.onblocked = () => {
				console.warn('Database upgrade blocked. Close other tabs using this database.');
			};
		});
	}

	/**
	 * Ensure database is ready before operations
	 */
	async ensureDB() {
		if (!this.db) {
			await this.initPromise;
		}
		return this.db as IDBDatabase;
	}

	/**
	 * Set a key-value pair with optional TTL
	 * @param {string} key - The key
	 * @param {*} value - The value (must be serializable)
	 * @param {number} ttl - Time to live in milliseconds (optional)
	 */
	async set<T = never>(key: string, value: T, ttl: number | null = null) {
		const db = await this.ensureDB();

		return new Promise((resolve, reject) => {
			const tx = db.transaction([this.storeName], 'readwrite');
			const store = tx.objectStore(this.storeName);

			const item = {
				key,
				value,
				expiry: ttl ? Date.now() + ttl : null,
				createdAt: Date.now(),
				updatedAt: Date.now()
			};

			const request = store.put(item);

			request.onsuccess = () => {
				resolve(value);
			};

			request.onerror = () => reject(new Error(`Failed to set key "${key}": ${request.error}`));

			tx.onerror = () => reject(new Error(`Transaction failed: ${tx.error}`));
		});
	}

	/**
	 * Get a value by key (returns null if expired or not found)
	 * @param {string} key - The key
	 * @param {boolean} skipExpiryCheck - Skip expiry check (for internal use)
	 */
	async get(key: string, skipExpiryCheck = false) {
		const db = await this.ensureDB();

		return new Promise((resolve, reject) => {
			const tx = db.transaction([this.storeName], 'readonly');
			const store = tx.objectStore(this.storeName);
			const request = store.get(key);

			request.onsuccess = async () => {
				const item = request.result;

				if (!item) {
					resolve(null);
					return;
				}

				// Check expiration
				if (!skipExpiryCheck && item.expiry && Date.now() > item.expiry) {
					await this.delete(key);
					resolve(null);
					return;
				}

				resolve(item.value);
			};

			request.onerror = () => reject(new Error(`Failed to get key "${key}": ${request.error}`));
		});
	}

	/**
	 * Delete a key
	 * @param {string} key - The key to delete
	 */
	async delete(key: string) {
		const db = await this.ensureDB();

		return new Promise((resolve, reject) => {
			const tx = db.transaction([this.storeName], 'readwrite');
			const store = tx.objectStore(this.storeName);
			const request = store.delete(key);

			request.onsuccess = () => {
				resolve(true);
			};

			request.onerror = () => reject(new Error(`Failed to delete key "${key}": ${request.error}`));
		});
	}

	/**
	 * Check if a key exists and is not expired
	 * @param {string} key - The key
	 */
	async has(key: string) {
		const value = await this.get(key);
		return value !== null;
	}

	/**
	 * Update TTL for an existing key without changing the value
	 * @param {string} key - The key
	 * @param {number} ms - New TTL in milliseconds
	 */
	async touch(key: string, ms: number) {
		await this.ensureDB();

		const value = await this.get(key);
		if (value === null) {
			throw new Error(`Key "${key}" does not exist`);
		}

		return this.set(key, value, ms);
	}

	/**
	 * Get all keys (optionally filter expired)
	 * @param {boolean} includeExpired - Include expired keys
	 */
	async keys(includeExpired = false) {
		const db = await this.ensureDB();

		return new Promise<IDBValidKey[]>((resolve, reject) => {
			const tx = db.transaction([this.storeName], 'readonly');
			const store = tx.objectStore(this.storeName);
			const request = store.getAllKeys();

			request.onsuccess = async () => {
				let keys = request.result;

				if (!includeExpired) {
					// Filter out expired keys
					const validKeys = [];
					for (const key of keys) {
						if (await this.has(key.toString())) {
							validKeys.push(key);
						}
					}
					keys = validKeys;
				}

				resolve(keys);
			};

			request.onerror = () => reject(new Error(`Failed to get keys: ${request.error}`));
		});
	}

	/**
	 * Clear all entries from the store
	 */
	async clear() {
		const db = await this.ensureDB();

		return new Promise((resolve, reject) => {
			const tx = db.transaction([this.storeName], 'readwrite');
			const store = tx.objectStore(this.storeName);
			const request = store.clear();

			request.onsuccess = () => {
				resolve(true);
			};

			request.onerror = () => reject(new Error(`Failed to clear store: ${request.error}`));
		});
	}

	/**
	 * Get the count of non-expired items
	 */
	async size() {
		const keys = await this.keys();
		return keys.length;
	}

	/**
	 * Remove all expired entries
	 */
	async cleanup() {
		const db = await this.ensureDB();

		return new Promise((resolve, reject) => {
			const tx = db.transaction([this.storeName], 'readwrite');
			const store = tx.objectStore(this.storeName);
			const index = store.index('expiry');
			const now = Date.now();
			let deletedCount = 0;

			// Get all items with expiry <= now
			const range = IDBKeyRange.upperBound(now);
			const request = index.openCursor(range);

			request.onsuccess = (event) => {
				const cursor = (event.target as IDBRequest)?.result;
				if (cursor) {
					// Skip items with null expiry
					if (cursor.value.expiry !== null) {
						cursor.delete();
						deletedCount++;
					}
					cursor.continue();
				}
			};

			tx.oncomplete = () => {
				resolve(deletedCount);
			};

			tx.onerror = () => reject(new Error(`Cleanup failed: ${tx.error}`));
		});
	}

	/**
	 * Start automatic cleanup task
	 * @param {number} intervalMs - Cleanup interval in milliseconds (default: 1 minute)
	 */
	startCleanupTask(intervalMs = 60000) {
		if (this.cleanupInterval) {
			clearInterval(this.cleanupInterval);
		}

		this.cleanupInterval = setInterval(async () => {
			try {
				await this.cleanup();
			} catch (error) {
				console.error('Cleanup task failed:', error);
			}
		}, intervalMs);
	}

	/**
	 * Stop automatic cleanup task
	 */
	stopCleanupTask() {
		if (this.cleanupInterval) {
			clearInterval(this.cleanupInterval);
			this.cleanupInterval = null;
		}
	}

	share() {
		return KV.set(this);
	}

	static get() {
		return getContext<KV | undefined>(CONTEXT_KEY);
	}

	static set(kv: KV) {
		return setContext(CONTEXT_KEY, kv);
	}
}

export class KVSync {
	kv: KV;

	constructor(kv: KV) {
		this.kv = kv;
	}

	async get<T = never>(key: string): Promise<T>;
	async get<T = never>(key: string, fetcher: () => Promise<T>, ms?: number | null): Promise<T>;
	async get<T = never>(key: string, fetcher?: () => Promise<T>, ms: number | null = null) {
		const data = await this.kv.get(key);

		if (data) {
			return data;
		} else if (fetcher) {
			return this.set(key, fetcher, ms);
		}

		return undefined;
	}

	async set<T = never>(key: string, fetcher: () => Promise<T>, ms: number | null = null) {
		const data = await fetcher();
		return await this.kv.set(key, data, ms);
	}
}
