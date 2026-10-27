import { api } from './client.js';
import type { SSOStatus } from './types.gen.js';

export const ssoKeys = { status: ['sso'] as const };

export const ssoApi = {
	status: () => api.get<SSOStatus>('/auth/sso')
};
