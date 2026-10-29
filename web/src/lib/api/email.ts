import { api } from './client.js';
import type { EmailStatus, UpdateEmailRequest } from './types.gen.js';

export const emailKeys = { status: ['email'] as const };

export const emailApi = {
	status: () => api.get<EmailStatus>('/email'),
	update: (body: UpdateEmailRequest) => api.patch<EmailStatus>('/email', body),
	test: (to: string) => api.post<EmailStatus>('/email/test', { to })
};
