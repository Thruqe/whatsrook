import type { AuthenticationState } from "../Types/index.ts";

// Covers every type both bun:sqlite and better-sqlite3 accept as a binding
type SqlBinding =
	| string
	| number
	| bigint
	| boolean
	| null
	| Uint8Array
	| Buffer;

interface DbStatement {
	run(...args: SqlBinding[]): unknown;
	get(...args: SqlBinding[]): unknown;
	all(...args: SqlBinding[]): unknown[];
}

interface DbAdapter {
	exec(sql: string): void;
	prepare(sql: string): DbStatement;
	close(): void;
}

async function openDatabase(dbPath: string): Promise<DbAdapter> {
	if (typeof globalThis.Bun !== "undefined") {
		const { Database } = await import("bun:sqlite");
		const db = new Database(dbPath, { create: true });
		return {
			exec: (sql: string) => db.exec(sql),
			prepare: (sql: string) => {
				const stmt = db.prepare(sql);
				return {
					run: (...args: SqlBinding[]) =>
						stmt.run(...(args as Parameters<typeof stmt.run>)),
					get: (...args: SqlBinding[]) =>
						stmt.get(...(args as Parameters<typeof stmt.get>)),
					all: (...args: SqlBinding[]) =>
						stmt.all(...(args as Parameters<typeof stmt.all>))
				};
			},
			close: () => db.close()
		};
	}

	// Node.js — requires: npm i better-sqlite3
	const BetterSqlite3 = (await import("better-sqlite3")).default;
	const db = new BetterSqlite3(dbPath);
	return {
		exec: (sql: string) => db.exec(sql),
		prepare: (sql: string) => {
			const stmt = db.prepare(sql);
			return {
				run: (...args: SqlBinding[]) => stmt.run(...args),
				get: (...args: SqlBinding[]) => stmt.get(...args),
				all: (...args: SqlBinding[]) => stmt.all(...args)
			};
		},
		close: () => db.close()
	};
}

// ---------------------------------------------------------------------------
// Bridge store
// ---------------------------------------------------------------------------

/**
 * Creates a SQLite-backed store for the WASM bridge.
 *
 * Each unique store name the bridge passes at runtime becomes its own table:
 *   CREATE TABLE "<store>" (key TEXT PRIMARY KEY, value BLOB NOT NULL)
 *
 * Tables are created on first access and their prepared statements are cached,
 * so subsequent calls pay zero DDL overhead.
 *
 * Behaviour preserved from the file-based implementation:
 *  - Write-through in-memory LRU cache (5 000 entries)
 *  - Critical stores (session / identity / device) are written synchronously
 *  - Non-critical writes are coalesced with a 50 ms debounce
 *  - flush() drains all pending debounced writes
 */
export async function useBridgeStore(
	dbFile = "auth.db"
): Promise<NonNullable<AuthenticationState["store"]>> {
	const db = await openDatabase(dbFile);

	db.exec(`PRAGMA journal_mode = WAL`);
	db.exec(`PRAGMA synchronous = NORMAL`);

	// ------------------------------------------------------------------
	// Dynamic per-store table management
	// ------------------------------------------------------------------

	// Guard against SQL injection: store names come from the WASM bridge
	// and are interpolated into DDL, so we restrict to safe identifiers.
	const SAFE_TABLE_NAME = /^[a-zA-Z_][a-zA-Z0-9_]*$/;

	interface StoreStatements {
		get: DbStatement;
		set: DbStatement;
		del: DbStatement;
	}

	const stmtCache = new Map<string, StoreStatements>();

	const ensureTable = (store: string): StoreStatements => {
		const hit = stmtCache.get(store);
		if (hit) return hit;

		if (!SAFE_TABLE_NAME.test(store)) {
			throw new Error(
				`Invalid store name: "${store}". Only alphanumeric characters and underscores are allowed.`
			);
		}

		db.exec(`
			CREATE TABLE IF NOT EXISTS "${store}" (
				key   TEXT NOT NULL PRIMARY KEY,
				value BLOB NOT NULL
			)
		`);

		const stmts: StoreStatements = {
			get: db.prepare(`SELECT value FROM "${store}" WHERE key = ?`),
			set: db.prepare(
				`INSERT INTO "${store}" (key, value) VALUES (?, ?)
				 ON CONFLICT (key) DO UPDATE SET value = excluded.value`
			),
			del: db.prepare(`DELETE FROM "${store}" WHERE key = ?`)
		};

		stmtCache.set(store, stmts);
		return stmts;
	};

	// ------------------------------------------------------------------
	// LRU write-through cache
	// ------------------------------------------------------------------

	const MAX_CACHE_ENTRIES = 5_000;
	const cache = new Map<string, Uint8Array>();

	const touchCache = (cacheKey: string, value: Uint8Array) => {
		cache.delete(cacheKey);
		cache.set(cacheKey, value);
		if (cache.size > MAX_CACHE_ENTRIES) {
			cache.delete(cache.keys().next().value!);
		}
	};

	// ------------------------------------------------------------------
	// Debounced write queue for non-critical stores
	// ------------------------------------------------------------------

	const WRITE_DELAY_MS = 50;

	interface Pending {
		store: string;
		key: string;
		value: Uint8Array;
		timer: ReturnType<typeof setTimeout>;
	}

	const pendingWrites = new Map<string, Pending>();

	const flushWrite = (cacheKey: string) => {
		const pending = pendingWrites.get(cacheKey);
		if (!pending) return;
		clearTimeout(pending.timer);
		pendingWrites.delete(cacheKey);
		ensureTable(pending.store).set.run(pending.key, pending.value);
	};

	const flushAll = () => {
		for (const cacheKey of pendingWrites.keys()) {
			flushWrite(cacheKey);
		}
	};

	// ------------------------------------------------------------------
	// Store implementation
	// ------------------------------------------------------------------

	return {
		async get(store: string, key: string): Promise<Uint8Array | null> {
			const cacheKey = `${store}\0${key}`;

			const cached = cache.get(cacheKey);
			if (cached) return cached;

			const pending = pendingWrites.get(cacheKey);
			if (pending) return pending.value;

			const row = ensureTable(store).get.get(key) as
				| { value: Uint8Array }
				| undefined;
			if (!row) return null;

			// Normalise: better-sqlite3 returns Buffer, bun:sqlite returns Uint8Array
			const arr =
				row.value instanceof Uint8Array
					? row.value
					: new Uint8Array(
							(row.value as Buffer).buffer,
							(row.value as Buffer).byteOffset,
							(row.value as Buffer).byteLength
						);

			touchCache(cacheKey, arr);
			return arr;
		},

		async set(store: string, key: string, value: Uint8Array): Promise<void> {
			const cacheKey = `${store}\0${key}`;

			// Skip identical writes
			const prev = cache.get(cacheKey);
			if (
				prev &&
				prev.length === value.length &&
				prev.every((b, i) => b === value[i])
			) {
				return;
			}

			touchCache(cacheKey, value);

			// Critical stores: write immediately, bypass debounce
			const critical =
				store === "session" || store === "identity" || store === "device";
			if (critical) {
				const existing = pendingWrites.get(cacheKey);
				if (existing) {
					clearTimeout(existing.timer);
					pendingWrites.delete(cacheKey);
				}
				ensureTable(store).set.run(key, value);
				return;
			}

			// Non-critical: coalesce rapid writes
			const existing = pendingWrites.get(cacheKey);
			if (existing) {
				clearTimeout(existing.timer);
			}

			const timer = setTimeout(() => flushWrite(cacheKey), WRITE_DELAY_MS);
			if (typeof timer === "object" && "unref" in timer) {
				(timer as NodeJS.Timeout).unref();
			}
			pendingWrites.set(cacheKey, { store, key, value, timer });
		},

		async delete(store: string, key: string): Promise<void> {
			const cacheKey = `${store}\0${key}`;
			cache.delete(cacheKey);

			const existing = pendingWrites.get(cacheKey);
			if (existing) {
				clearTimeout(existing.timer);
				pendingWrites.delete(cacheKey);
			}

			ensureTable(store).del.run(key);
		},

		/**
		 * Flush all pending debounced writes and optionally close the DB.
		 * Pass `{ close: true }` during shutdown to release the file handle.
		 */
		async flush(opts?: { close?: boolean }): Promise<void> {
			flushAll();
			if (opts?.close) db.close();
		}
	};
}
