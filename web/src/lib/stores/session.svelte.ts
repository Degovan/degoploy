import { api, ApiError, setCsrfToken } from '#lib/api/client.js';
import type { Action, Me } from '#lib/api/types.gen.js';

/** The signed-in user, their CSRF token and global permissions. */
class Session {
	me = $state<Me | null>(null);
	loaded = $state(false);

	#inflight: Promise<void> | null = null;

	get user() {
		return this.me?.user ?? null;
	}

	can(action: Action): boolean {
		return this.me?.permissions.includes(action) ?? false;
	}

	set(me: Me | null) {
		this.me = me;
		this.loaded = true;
		setCsrfToken(me?.csrf_token ?? '');
	}

	/** Loads /me once; later calls reuse the result until `clear()`. */
	ensure(): Promise<void> {
		if (this.loaded) return Promise.resolve();
		this.#inflight ??= api
			.get<Me>('/me')
			.then((me) => this.set(me))
			.catch((err) => {
				if (err instanceof ApiError && err.status === 401) {
					this.set(null);
					return;
				}
				throw err;
			})
			.finally(() => {
				this.#inflight = null;
			});
		return this.#inflight;
	}

	async logout() {
		try {
			await api.post('/auth/logout');
		} finally {
			this.clear();
		}
	}

	clear() {
		this.set(null);
	}
}

export const session = new Session();
