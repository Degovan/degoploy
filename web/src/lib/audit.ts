import type { AuditEntry } from '#lib/api/types.gen.js';

/** Activity categories, matched by action prefix (the API's `action` filter). */
export const auditCategories = [
	{ value: 'auth.', label: 'Sign-in' },
	{ value: 'project.', label: 'Projects' },
	{ value: 'rule.', label: 'Trigger rules' },
	{ value: 'variable.', label: 'Variables' },
	{ value: 'deployment.', label: 'Deployments' },
	{ value: 'user.', label: 'Users' },
	{ value: 'invite.', label: 'Invites' },
	{ value: 'sso.', label: 'Sign-in methods' },
	{ value: 'email.', label: 'Email' }
] as const;

export type AuditCategory = (typeof auditCategories)[number]['value'];

export function categoryOf(action: string): AuditCategory | null {
	return auditCategories.find((c) => action.startsWith(c.value))?.value ?? null;
}

/**
 * One entry as a sentence: "{actor} {verb} {target} {suffix}", plus a meta line.
 * Targets in `mono` are identifiers (slugs, variable names, emails).
 */
export type AuditSentence = {
	actor: string;
	verb: string;
	target?: string;
	mono?: boolean;
	/** For targets that are pages: project or deployment. */
	link?: { project: string; deployment?: string };
	suffix?: string;
	meta: string[];
};

const roleLabel = (r: unknown) =>
	typeof r === 'string' && r ? r.charAt(0).toUpperCase() + r.slice(1) : '';

const str = (v: unknown) => (typeof v === 'string' ? v : v == null ? '' : String(v));

/**
 * `names` maps usernames to display names (for the actor and user targets);
 * unknown names fall back to @username.
 */
export function describe(e: AuditEntry, names: Map<string, string>): AuditSentence {
	const d = e.detail;
	const person = (username: string) => names.get(username) || (username ? `@${username}` : '');
	const actor =
		person(e.username) ||
		(e.action === 'auth.login_failed' && str(d.username)
			? `@${str(d.username)}`
			: e.user_id
				? 'Deleted user'
				: 'Someone');
	const slug = str(d.slug);
	const project = e.target_type === 'project' ? e.target_id : '';
	const inProject = (verb: string, target: string, suffix?: string): AuditSentence => ({
		actor,
		verb,
		target,
		mono: true,
		suffix,
		meta: []
	});
	const onProject = (verb: string): AuditSentence => ({
		actor,
		verb,
		target: slug || `project #${e.target_id}`,
		mono: true,
		link: project && e.action !== 'project.delete' ? { project } : undefined,
		meta: []
	});

	switch (e.action) {
		case 'auth.login':
			return { actor, verb: 'signed in', meta: [`provider: ${str(d.provider) || 'local'}`] };
		case 'auth.login_failed':
			return {
				actor,
				verb: 'failed to sign in',
				meta: [str(d.reason), d.provider ? `provider: ${str(d.provider)}` : ''].filter(Boolean)
			};
		case 'auth.logout':
			return { actor, verb: 'signed out', meta: [] };
		case 'auth.password_change':
			return { actor, verb: 'changed their password', meta: [] };

		case 'project.create':
			return onProject('created project');
		case 'project.update': {
			const s = onProject('updated the settings of');
			s.meta = Object.keys((d.changes as object) ?? {});
			return s;
		}
		case 'project.delete':
			return { ...onProject('deleted project'), meta: d.name ? [str(d.name)] : [] };
		case 'project.secrets_view':
			return onProject('viewed the secrets of');
		case 'project.key_regenerate':
			return onProject('regenerated the deploy key for');
		case 'project.secret_regenerate':
			return onProject('regenerated the webhook secret for');
		case 'project.script_update':
			return { ...onProject('edited the deploy script of'), meta: [`revision ${str(d.revision)}`] };

		case 'rule.create':
			return inProject('added trigger rule', str(d.rule), `in ${slug}`);
		case 'rule.update':
			return inProject('updated trigger rule', str(d.rule), `in ${slug}`);
		case 'rule.delete':
			return inProject('deleted trigger rule', str(d.rule), `from ${slug}`);
		case 'rule.reorder':
			return onProject('reordered the trigger rules of');

		case 'variable.create':
		case 'variable.update':
		case 'variable.delete': {
			const [verb, prep] = {
				'variable.create': ['added variable', 'to'],
				'variable.update': ['updated variable', 'in'],
				'variable.delete': ['deleted variable', 'from']
			}[e.action];
			const s = inProject(verb, str(d.name), `${prep} ${slug}`);
			s.meta = [
				d.secret ? 'secret' : '',
				d.renamed_from ? `renamed from ${str(d.renamed_from)}` : '',
				d.value_changed ? 'value changed' : ''
			].filter(Boolean);
			return s;
		}

		case 'deployment.manual':
			return {
				actor,
				verb: 'deployed',
				target: `${slug} #${str(d.deployment)}`,
				mono: true,
				link: project ? { project, deployment: str(d.deployment) } : undefined,
				meta: d.ref ? [str(d.ref)] : []
			};
		case 'deployment.redeploy':
			return {
				actor,
				verb: 'redeployed',
				target: `${slug} #${str(d.from)}`,
				mono: true,
				link: project ? { project, deployment: str(d.deployment) } : undefined,
				meta: [`as #${str(d.deployment)}`]
			};
		case 'deployment.cancel':
			return {
				actor,
				verb: 'canceled deployment',
				target: `#${e.target_id}`,
				mono: true,
				meta: []
			};

		case 'user.create':
			return {
				actor,
				verb: 'added',
				target: person(str(d.username)),
				suffix: `as ${roleLabel(d.role)}`,
				meta: []
			};
		case 'user.update': {
			const changes = (d.changes ?? {}) as Record<string, [unknown, unknown]>;
			const target = person(str(d.username));
			const rest = Object.keys(changes).filter((k) => k !== 'role' && k !== 'disabled');
			if (changes.role) {
				return {
					actor,
					verb: 'changed the role of',
					target,
					suffix: `${roleLabel(changes.role[0])} → ${roleLabel(changes.role[1])}`,
					meta: rest
				};
			}
			if (changes.disabled) {
				return { actor, verb: changes.disabled[1] ? 'disabled' : 'enabled', target, meta: rest };
			}
			return { actor, verb: 'updated', target, meta: rest };
		}
		case 'user.delete':
			return { actor, verb: 'deleted user', target: `@${str(d.username)}`, mono: true, meta: [] };
		case 'user.password_reset':
			return {
				actor,
				verb: 'reset the password of',
				target: person(str(d.username)),
				meta: d.require_change ? ['must choose a new one at next sign-in'] : []
			};

		case 'invite.create':
			return {
				actor,
				verb: 'invited',
				target: str(d.email),
				mono: true,
				suffix: `as ${roleLabel(d.role)}`,
				meta: []
			};
		case 'invite.renew':
			return {
				actor,
				verb: 'renewed the invite link for',
				target: str(d.email),
				mono: true,
				meta: []
			};
		case 'invite.revoke':
			return { actor, verb: 'revoked the invite for', target: str(d.email), mono: true, meta: [] };
		case 'sso.update':
			return {
				actor,
				verb: 'changed the sign-in settings',
				meta: Array.isArray(d.changed) ? (d.changed as string[]) : []
			};
		case 'email.update':
			return {
				actor,
				verb: 'changed the email settings',
				meta: Array.isArray(d.changed) ? (d.changed as string[]) : []
			};
		case 'email.test':
			return {
				actor,
				verb: 'sent a test email to',
				target: str(d.to),
				mono: true,
				meta: [d.delivered ? 'delivered' : 'failed']
			};
		case 'invite.accept':
			return { actor, verb: 'accepted an invite', suffix: `as ${roleLabel(d.role)}`, meta: [] };
	}
	return { actor, verb: e.action, target: e.target_id || undefined, mono: true, meta: [] };
}

/** Consecutive entries collapse when the same person repeats the same action on the same target. */
export const sameEvent = (a: AuditEntry, b: AuditEntry) =>
	a.user_id === b.user_id &&
	a.action === b.action &&
	a.target_type === b.target_type &&
	a.target_id === b.target_id &&
	JSON.stringify(a.detail) === JSON.stringify(b.detail);
