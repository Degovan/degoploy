import type { Provider } from '#lib/api/types.gen.js';

export type RepoInfo = { host: string; path: string; web: string };

/** Parses git@host:owner/repo.git, ssh://… and https://… remotes into a browsable URL. */
export function parseRepo(url: string): RepoInfo | null {
	url = url.trim();
	let host: string, path: string;
	const scp = /^[^@\s/]+@([^:\s/]+):(.+)$/.exec(url);
	if (scp) {
		[, host, path] = scp as unknown as [string, string, string];
	} else {
		try {
			const u = new URL(url);
			if (!['ssh:', 'https:', 'http:'].includes(u.protocol)) return null;
			host = u.hostname;
			path = u.pathname;
		} catch {
			return null;
		}
	}
	path = path
		.replace(/^\/+/, '')
		.replace(/\.git$/, '')
		.replace(/\/+$/, '');
	if (!path.includes('/')) return null;
	return { host, path, web: `https://${host}/${path}` };
}

/** Where to add the deploy key and webhook in the provider's UI. */
export function providerLinks(provider: Provider, repoUrl: string) {
	const repo = parseRepo(repoUrl);
	if (!repo) return null;
	return provider === 'github'
		? {
				repo: repo.web,
				deployKeys: `${repo.web}/settings/keys/new`,
				webhooks: `${repo.web}/settings/hooks/new`
			}
		: {
				repo: repo.web,
				deployKeys: `${repo.web}/settings/keys`,
				webhooks: `${repo.web}/settings/hooks`
			};
}

export const providerLabel: Record<Provider, string> = { github: 'GitHub', gitea: 'Gitea' };

/** Lowercase dash-separated slug derived from a display name. */
export function slugify(name: string): string {
	return name
		.toLowerCase()
		.normalize('NFKD')
		.replace(/[̀-ͯ]/g, '')
		.replace(/[^a-z0-9]+/g, '-')
		.replace(/^-+|-+$/g, '')
		.slice(0, 63);
}
