import { api } from './client.js';
import type { OIDCDiscovery, SSOStatus, UpdateSSORequest } from './types.gen.js';

export const ssoKeys = {
	status: ['sso'] as const,
	discovery: (issuer: string) => ['sso', 'discovery', issuer] as const
};

export const ssoApi = {
	status: () => api.get<SSOStatus>('/auth/sso'),
	update: (body: UpdateSSORequest) => api.patch<SSOStatus>('/auth/sso', body),
	discover: (issuer: string) => api.post<OIDCDiscovery>('/auth/sso/discover', { issuer })
};
