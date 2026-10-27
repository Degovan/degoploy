import { api } from './client.js';
import type {
	RuleInput,
	RuleTestRequest,
	RuleTestResponse,
	TriggerRule,
	WebhookEvent
} from './types.gen.js';

export const ruleKeys = {
	list: (projectId: number) => ['projects', projectId, 'rules'] as const,
	events: (projectId: number) => ['projects', projectId, 'webhook-events'] as const
};

const base = (projectId: number) => `/projects/${projectId}/rules`;

export const rulesApi = {
	list: (projectId: number) => api.get<TriggerRule[]>(base(projectId)),
	create: (projectId: number, body: RuleInput) => api.post<TriggerRule>(base(projectId), body),
	update: (projectId: number, id: number, body: RuleInput) =>
		api.put<TriggerRule>(`${base(projectId)}/${id}`, body),
	remove: (projectId: number, id: number) => api.delete<void>(`${base(projectId)}/${id}`),
	reorder: (projectId: number, ids: number[]) =>
		api.put<TriggerRule[]>(`${base(projectId)}/order`, { ids }),
	test: (projectId: number, body: RuleTestRequest) =>
		api.post<RuleTestResponse>(`${base(projectId)}/test`, body),
	events: (projectId: number) => api.get<WebhookEvent[]>(`/projects/${projectId}/webhook-events`)
};
