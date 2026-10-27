// Thin fetch wrapper for /api/v1: JSON in/out, CSRF header, typed errors.

const BASE = '/api/v1';

export class ApiError extends Error {
	constructor(
		readonly status: number,
		readonly code: string,
		message: string,
		/** Input field rejected by validation (HTTP 422), if any. */
		readonly field?: string
	) {
		super(message);
		this.name = 'ApiError';
	}
}

let csrfToken = '';
let onUnauthorized: (() => void) | undefined;

export function setCsrfToken(token: string) {
	csrfToken = token;
}

/** Called when an authenticated request comes back 401 (session expired/revoked). */
export function setUnauthorizedHandler(fn: () => void) {
	onUnauthorized = fn;
}

type Method = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';

async function request<T>(method: Method, path: string, body?: unknown): Promise<T> {
	const headers: Record<string, string> = { Accept: 'application/json' };
	if (body !== undefined) headers['Content-Type'] = 'application/json';
	if (method !== 'GET' && csrfToken) headers['X-CSRF-Token'] = csrfToken;

	let res: Response;
	try {
		res = await fetch(BASE + path, {
			method,
			headers,
			body: body === undefined ? undefined : JSON.stringify(body),
			credentials: 'same-origin'
		});
	} catch {
		throw new ApiError(0, 'network', 'Cannot reach the degoploy server');
	}

	if (res.status === 204) return undefined as T;

	const data = await res.json().catch(() => null);
	if (!res.ok) {
		if (res.status === 401 && path !== '/auth/login') onUnauthorized?.();
		throw new ApiError(
			res.status,
			data?.code ?? 'http_' + res.status,
			data?.error ?? res.statusText ?? 'Request failed',
			data?.field
		);
	}
	return data as T;
}

export const api = {
	get: <T>(path: string) => request<T>('GET', path),
	post: <T>(path: string, body?: unknown) => request<T>('POST', path, body),
	put: <T>(path: string, body?: unknown) => request<T>('PUT', path, body),
	patch: <T>(path: string, body?: unknown) => request<T>('PATCH', path, body),
	delete: <T>(path: string) => request<T>('DELETE', path)
};

/** Human-readable message for any thrown value. */
export function errorMessage(err: unknown): string {
	if (err instanceof Error) return err.message;
	return 'Something went wrong';
}
