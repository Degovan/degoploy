import { api } from './client.js';
import type { Variable, VariableInput } from './types.gen.js';

export const variableKeys = {
	list: (projectId: number) => ['projects', projectId, 'variables'] as const
};

const base = (projectId: number) => `/projects/${projectId}/variables`;

export const variablesApi = {
	list: (projectId: number) => api.get<Variable[]>(base(projectId)),
	create: (projectId: number, body: VariableInput) => api.post<Variable>(base(projectId), body),
	update: (projectId: number, id: number, body: VariableInput) =>
		api.put<Variable>(`${base(projectId)}/${id}`, body),
	remove: (projectId: number, id: number) => api.delete<void>(`${base(projectId)}/${id}`)
};
