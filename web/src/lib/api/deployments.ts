import type { QueryClient } from '@tanstack/svelte-query';
import { api } from './client.js';
import type { Deployment, DeploymentNeighbors, LogTimes } from './types.gen.js';

export const deploymentKeys = {
	list: (projectId: number) => ['projects', projectId, 'deployments'] as const,
	recent: ['deployments', 'recent'] as const,
	active: ['deployments', 'recent', 'active'] as const,
	detail: (id: number) => ['deployments', id] as const,
	// Under the project's list key, so a new deployment refreshes it.
	neighbors: (projectId: number, id: number) =>
		['projects', projectId, 'deployments', 'neighbors', id] as const
};

export const deploymentsApi = {
	list: (projectId: number, limit = 20) =>
		api.get<Deployment[]>(`/projects/${projectId}/deployments?limit=${limit}`),
	recent: (limit = 10) => api.get<Deployment[]>(`/deployments?limit=${limit}`),
	/** Queued and running deployments of all projects. */
	active: () => api.get<Deployment[]>('/deployments?status=active&limit=100'),
	get: (id: number) => api.get<Deployment>(`/deployments/${id}`),
	deploy: (projectId: number, ref: string) =>
		api.post<Deployment>(`/projects/${projectId}/deployments`, { ref }),
	cancel: (id: number) => api.post<Deployment>(`/deployments/${id}/cancel`),
	redeploy: (id: number) => api.post<Deployment>(`/deployments/${id}/redeploy`),
	neighbors: (id: number) => api.get<DeploymentNeighbors>(`/deployments/${id}/neighbors`),
	logTimes: (id: number) => api.get<LogTimes>(`/deployments/${id}/log/times`),
	logUrl: (id: number) => `/api/v1/deployments/${id}/log?download=1`
};

export const triggerLabels: Record<Deployment['trigger'], string> = {
	webhook: 'Webhook',
	manual: 'Manual',
	redeploy: 'Redeploy',
	rollback: 'Rollback'
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
