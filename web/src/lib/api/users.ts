import { api } from './client.js';
import type {
	AuditPage,
	AcceptInviteRequest,
	ChangePasswordRequest,
	CreateInvitesRequest,
	CreateUserRequest,
	Invite,
	InviteLink,
	InvitePreview,
	Me,
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

export const inviteKeys = { all: ['invites'] as const };

export const invitesApi = {
	list: () => api.get<Invite[]>('/invites'),
	create: (body: CreateInvitesRequest) => api.post<InviteLink[]>('/invites', body),
	renew: (id: number) => api.post<InviteLink>(`/invites/${id}/renew`),
	revoke: (id: number) => api.delete<void>(`/invites/${id}`),
	preview: (token: string) => api.get<InvitePreview>(`/auth/invite/${encodeURIComponent(token)}`),
	accept: (token: string, body: AcceptInviteRequest) =>
		api.post<Me>(`/auth/invite/${encodeURIComponent(token)}`, body)
};

/** The accept page for an invite token, on this server. */
export const inviteUrl = (token: string) => `${location.origin}/invite/${token}`;

export type AuditQuery = { action?: string; user?: number; before?: number; limit?: number };

export const auditKeys = { list: (q: AuditQuery) => ['audit', q] as const };

export const auditApi = {
	list: (q: AuditQuery) => {
		const params = new URLSearchParams();
		for (const [k, v] of Object.entries(q)) if (v) params.set(k, String(v));
		return api.get<AuditPage>(`/audit?${params}`);
	}
};
