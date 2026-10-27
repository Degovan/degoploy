import { api } from './client.js';
import type {
	AuditPage,
	ChangePasswordRequest,
	CreateUserRequest,
	UpdateUserRequest,
	User
} from './types.gen.js';

export const userKeys = { all: ['users'] as const };

export const usersApi = {
	list: () => api.get<User[]>('/users'),
	create: (body: CreateUserRequest) => api.post<User>('/users', body),
	update: (id: number, body: UpdateUserRequest) => api.patch<User>(`/users/${id}`, body),
	setPassword: (id: number, password: string) =>
		api.put<void>(`/users/${id}/password`, { password }),
	remove: (id: number) => api.delete<void>(`/users/${id}`),
	changeOwnPassword: (body: ChangePasswordRequest) => api.post<void>('/me/password', body)
};

export type AuditQuery = { action?: string; user?: number; before?: number; limit?: number };

export const auditKeys = { list: (q: AuditQuery) => ['audit', q] as const };

export const auditApi = {
	list: (q: AuditQuery) => {
		const params = new URLSearchParams();
		for (const [k, v] of Object.entries(q)) if (v) params.set(k, String(v));
		return api.get<AuditPage>(`/audit?${params}`);
	}
};
