import type { QueryClient } from '@tanstack/svelte-query';
import { api } from './client.js';
import type { Deployment } from './types.gen.js';

export const deploymentKeys = {
	list: (projectId: number) => ['projects', projectId, 'deployments'] as const,
	detail: (id: number) => ['deployments', id] as const
};

export const deploymentsApi = {
	list: (projectId: number, limit = 20) =>
		api.get<Deployment[]>(`/projects/${projectId}/deployments?limit=${limit}`),
	get: (id: number) => api.get<Deployment>(`/deployments/${id}`),
	deploy: (projectId: number, ref: string) =>
		api.post<Deployment>(`/projects/${projectId}/deployments`, { ref }),
	cancel: (id: number) => api.post<Deployment>(`/deployments/${id}/cancel`),
	redeploy: (id: number) => api.post<Deployment>(`/deployments/${id}/redeploy`),
	logUrl: (id: number) => `/api/v1/deployments/${id}/log?download=1`
};

export const isActive = (d: Pick<Deployment, 'status'>) =>
	d.status === 'queued' || d.status === 'running';

/** "main" for refs/heads/main, "v1.0" for refs/tags/v1.0. */
export const shortRef = (ref: string) => ref.replace(/^refs\/(heads|tags)\//, '');

// queued → running → finished; a deployment never moves backwards.
const stage = (d: Pick<Deployment, 'status'>) =>
	d.status === 'queued' ? 0 : d.status === 'running' ? 1 : 2;

/**
 * Stores a deployment in the query cache unless it is older than what is cached.
 * HTTP responses and realtime events race, so a response saying "queued" may
 * arrive after the event saying "running"; the later stage always wins.
 */
export function cacheDeployment(queryClient: QueryClient, d: Deployment) {
	const key = deploymentKeys.detail(d.id);
	const current = queryClient.getQueryData<Deployment>(key);
	if (current && stage(current) > stage(d)) return;
	queryClient.setQueryData(key, d);
}
