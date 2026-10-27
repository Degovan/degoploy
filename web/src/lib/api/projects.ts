import { api } from './client.js';
import type {
	ConnectionTest,
	CreateProjectRequest,
	Project,
	ProjectSecrets,
	Script,
	ScriptRevision,
	UpdateProjectRequest
} from './types.gen.js';

export const projectKeys = {
	all: ['projects'] as const,
	detail: (id: number) => ['projects', id] as const,
	secrets: (id: number) => ['projects', id, 'secrets'] as const,
	script: (id: number) => ['projects', id, 'script'] as const,
	revisions: (id: number) => ['projects', id, 'script', 'revisions'] as const
};

const base = (id: number) => `/projects/${id}`;

export const projectsApi = {
	list: () => api.get<Project[]>('/projects'),
	get: (id: number) => api.get<Project>(base(id)),
	create: (body: CreateProjectRequest) => api.post<Project>('/projects', body),
	update: (id: number, body: UpdateProjectRequest) => api.patch<Project>(base(id), body),
	remove: (id: number) => api.delete<void>(base(id)),
	secrets: (id: number) => api.get<ProjectSecrets>(`${base(id)}/secrets`),
	regenerateKey: (id: number) => api.post<ProjectSecrets>(`${base(id)}/regenerate-key`),
	regenerateSecret: (id: number) => api.post<ProjectSecrets>(`${base(id)}/regenerate-secret`),
	testConnection: (id: number) => api.post<ConnectionTest>(`${base(id)}/test-connection`),
	script: (id: number) => api.get<Script>(`${base(id)}/script`),
	saveScript: (id: number, content: string) => api.put<Script>(`${base(id)}/script`, { content }),
	revisions: (id: number) => api.get<ScriptRevision[]>(`${base(id)}/script/revisions`)
};
