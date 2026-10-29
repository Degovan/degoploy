import { Marked, type Tokens } from 'marked';

// The user guide in docs/guide, bundled at build time so the docs always match
// the running version and work without internet access.
const files = import.meta.glob('../../../docs/guide/*.md', {
	query: '?raw',
	import: 'default',
	eager: true
}) as Record<string, string>;

/** Sidebar order and grouping; slugs are file names without .md ("" is README). */
export const docGroups: { title: string; icon: string; pages: string[] }[] = [
	{ title: 'Getting started', icon: 'rocket', pages: ['', 'getting-started'] },
	{
		title: 'Projects',
		icon: 'folder',
		pages: [
			'projects',
			'trigger-rules',
			'deploy-script',
			'variables-and-secrets',
			'runtimes',
			'deployments',
			'project-settings'
		]
	},
	{
		title: 'Users & access',
		icon: 'users',
		pages: ['users-and-invites', 'roles', 'single-sign-on', 'audit-log']
	},
	{ title: 'Configuration', icon: 'settings', pages: ['email'] },
	{ title: 'Your account', icon: 'user', pages: ['your-account'] }
];

export const docOrder = docGroups.flatMap((g) => g.pages);

const fileName = (slug: string) => (slug === '' ? 'README' : slug);
const source = (slug: string) =>
	Object.entries(files).find(([path]) => path.endsWith(`/${fileName(slug)}.md`))?.[1];

export type DocMeta = {
	slug: string;
	/** Short label for navigation (front matter `title`). */
	nav: string;
	/** The page's own heading. */
	title: string;
	/** The first paragraph, shown large under the title. */
	lead: string;
	group: string;
	admin: boolean;
	updated: string;
	minutes: number;
	/** The markdown without front matter, for "Copy page as Markdown". */
	markdown: string;
	/** Plain text, for search. */
	text: string;
};

function frontMatter(raw: string): { data: Record<string, string>; body: string } {
	const m = raw.match(/^---\n([\s\S]*?)\n---\n?/);
	if (!m) return { data: {}, body: raw };
	const data: Record<string, string> = {};
	for (const line of m[1]!.split('\n')) {
		const [k, ...v] = line.split(':');
		if (k && v.length) data[k.trim()] = v.join(':').trim();
	}
	return { data, body: raw.slice(m[0].length) };
}

const plain = (md: string) =>
	md
		.replace(/```[\s\S]*?```/g, ' ')
		.replace(/<[^>]+>/g, ' ')
		.replace(/!?\[([^\]]*)\]\([^)]*\)/g, '$1')
		.replace(/[*_`>#|]/g, ' ')
		.replace(/\s+/g, ' ')
		.trim();

function metaOf(slug: string): DocMeta | undefined {
	const raw = source(slug);
	if (raw === undefined) return undefined;
	const { data, body } = frontMatter(raw);
	const title = body.match(/^# (.+)$/m)?.[1] ?? data.title ?? slug;
	const afterTitle = body.split(/^# .+$/m)[1] ?? '';
	const lead = afterTitle.trim().split(/\n\s*\n/)[0] ?? '';
	const text = plain(body);
	return {
		slug,
		nav: data.title ?? title,
		title,
		lead: lead.startsWith('#') || lead.startsWith('|') ? '' : lead,
		group: docGroups.find((g) => g.pages.includes(slug))?.title ?? '',
		admin: data.admin === 'true',
		updated: data.updated ?? '',
		minutes: Math.max(1, Math.round(text.split(' ').length / 200)),
		markdown: body.trim(),
		text
	};
}

export const docs: DocMeta[] = docOrder.map(metaOf).filter((d): d is DocMeta => !!d);
export const docBySlug = (slug: string) => docs.find((d) => d.slug === slug);

/** Heading ids as GitHub makes them, so in-page links work in both places. */
export function slugify(text: string): string {
	return text
		.toLowerCase()
		.replace(/<[^>]+>/g, '')
		.replace(/[^\p{L}\p{N}\s-]/gu, '')
		.trim()
		.replace(/\s/g, '-');
}

const REPO = 'https://github.com/Degovan/degoploy';

/** Where to suggest an edit to a page. */
export const editUrl = (slug: string) => `${REPO}/edit/main/docs/guide/${fileName(slug)}.md`;

/** Where a link in the markdown points: guide pages stay in the app. */
function rewriteHref(href: string): { href: string; external: boolean } {
	if (/^[a-z]+:/i.test(href)) return { href, external: true };
	if (href.startsWith('#')) return { href, external: false };
	const [path, hash] = href.split('#');
	const anchor = hash ? `#${hash}` : '';
	const page = path!.match(/^(?:\.\/)?([\w-]+)\.md$/);
	if (page) {
		const slug = page[1] === 'README' ? '' : page[1]!;
		return { href: `/docs${slug ? `/${slug}` : ''}${anchor}`, external: false };
	}
	// Other files (INSTALL.md) are for server admins; open them on GitHub.
	return {
		href: `${REPO}/blob/main/docs/${path!.replace(/^\.\.\//, '')}${anchor}`,
		external: true
	};
}

const escape = (s: string) =>
	s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');

// Rendered inline HTML back to plain text, for the page outline.
const entities: Record<string, string> = { amp: '&', lt: '<', gt: '>', quot: '"', '#39': "'" };
const textOf = (html: string) =>
	html.replace(/<[^>]+>/g, '').replace(/&(amp|lt|gt|quot|#39);/g, (m, e) => entities[e] ?? m);

// Lucide icon paths for callouts, inlined so the HTML needs no components.
const icon = (paths: string) =>
	`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${paths}</svg>`;
const callouts: Record<string, { label: string; icon: string }> = {
	note: {
		label: 'Note',
		icon: icon('<circle cx="12" cy="12" r="10"/><path d="M12 16v-4"/><path d="M12 8h.01"/>')
	},
	tip: {
		label: 'Tip',
		icon: icon(
			'<path d="M15 14c.2-1 .7-1.7 1.5-2.5 1-.9 1.5-2.2 1.5-3.5A6 6 0 0 0 6 8c0 1 .2 2.2 1.5 3.5.7.7 1.3 1.5 1.5 2.5"/><path d="M9 18h6"/><path d="M10 22h4"/>'
		)
	},
	important: {
		label: 'Important',
		icon: icon(
			'<path d="M22 17a2 2 0 0 1-2 2H6.828a2 2 0 0 0-1.414.586l-2.202 2.202A.71.71 0 0 1 2 21.286V5a2 2 0 0 1 2-2h16a2 2 0 0 1 2 2z"/><path d="M12 7v2"/><path d="M12 13h.01"/>'
		)
	},
	warning: {
		label: 'Warning',
		icon: icon(
			'<path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3"/><path d="M12 9v4"/><path d="M12 17h.01"/>'
		)
	},
	caution: {
		label: 'Caution',
		icon: icon(
			'<path d="M12 16h.01"/><path d="M12 8v4"/><path d="M15.312 2a2 2 0 0 1 1.414.586l4.688 4.688A2 2 0 0 1 22 8.688v6.624a2 2 0 0 1-.586 1.414l-4.688 4.688a2 2 0 0 1-1.414.586H8.688a2 2 0 0 1-1.414-.586l-4.688-4.688A2 2 0 0 1 2 15.312V8.688a2 2 0 0 1 .586-1.414l4.688-4.688A2 2 0 0 1 8.688 2z"/>'
		)
	}
};

const checkIcon =
	'<svg xmlns="http://www.w3.org/2000/svg" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>';

export type DocHeading = { id: string; text: string; depth: 2 | 3 };

type Parser = {
	parser: { parse: (t: Tokens.Generic[]) => string; parseInline: (t: Tokens.Generic[]) => string };
};

/**
 * Renders a page body to HTML. The title and lead are left out: the page
 * header shows them.
 */
export function renderDoc(slug: string): { html: string; headings: DocHeading[] } {
	const meta = docBySlug(slug);
	if (!meta) return { html: '', headings: [] };
	let body = meta.markdown.replace(/^# .+\n/m, '');
	if (meta.lead) body = body.replace(meta.lead, '');

	const headings: DocHeading[] = [];
	const marked = new Marked({ gfm: true });
	marked.use({
		renderer: {
			heading(this: Parser, t: Tokens.Heading) {
				const inner = this.parser.parseInline(t.tokens);
				const id = slugify(t.text);
				if (t.depth === 2 || t.depth === 3) {
					headings.push({ id, text: textOf(inner), depth: t.depth });
				}
				const anchor =
					t.depth === 2
						? `<a class="docs-anchor" href="#${id}" aria-label="Link to this section">#</a>`
						: '';
				return `<h${t.depth} id="${id}">${inner}${anchor}</h${t.depth}>\n`;
			},
			link(this: Parser, t: Tokens.Link) {
				const { href, external } = rewriteHref(t.href);
				const attrs = external ? ' target="_blank" rel="noreferrer"' : '';
				return `<a href="${escape(href)}"${attrs}>${this.parser.parseInline(t.tokens)}</a>`;
			},
			code(t: Tokens.Code) {
				if (t.lang === 'mermaid') {
					return `<div class="docs-diagram" data-mermaid="${escape(t.text)}"></div>\n`;
				}
				const lang = t.lang ? escape(t.lang) : 'text';
				return (
					`<div class="docs-code"><div class="docs-code-head"><span>${lang}</span>` +
					`<button type="button" class="docs-copy" data-copy>Copy</button></div>` +
					`<pre><code>${escape(t.text)}</code></pre></div>\n`
				);
			},
			table(this: Parser, t: Tokens.Table) {
				const cell = (c: Tokens.TableCell, tag: string) =>
					`<${tag}${c.align ? ` style="text-align:${c.align}"` : ''}>${this.parser.parseInline(c.tokens)}</${tag}>`;
				const head = `<tr>${t.header.map((c) => cell(c, 'th')).join('')}</tr>`;
				const body = t.rows
					.map((r) => {
						// A row with only its first cell filled is a group label inside the table.
						const group = r.slice(1).every((c) => !c.text.trim());
						return group
							? `<tr class="docs-row-group"><td colspan="${r.length}">${this.parser.parseInline(r[0]!.tokens)}</td></tr>`
							: `<tr>${r.map((c) => cell(c, 'td')).join('')}</tr>`;
					})
					.join('');
				return `<div class="docs-table"><table><thead>${head}</thead><tbody>${body}</tbody></table></div>\n`;
			},
			list(this: Parser, t: Tokens.List) {
				const task = t.items.some((i) => i.task);
				const items = t.items
					.map((i) => {
						const inner = this.parser.parse(i.tokens);
						return task
							? `<li class="docs-check${i.checked ? ' is-fact' : ''}"><label>${inner}</label></li>`
							: `<li>${inner}</li>`;
					})
					.join('\n');
				if (t.ordered) {
					const start = t.start !== 1 && t.start !== '' ? ` start="${t.start}"` : '';
					return `<ol class="docs-steps"${start}>\n${items}\n</ol>\n`;
				}
				return `<ul${task ? ' class="docs-checklist"' : ''}>\n${items}\n</ul>\n`;
			},
			// "- [ ]" items are a checklist the reader ticks off (remembered per page);
			// "- [x]" items state facts and show as already ticked.
			checkbox({ checked }: Tokens.Checkbox) {
				const attrs = checked ? ' checked disabled' : '';
				return (
					`<input type="checkbox" class="docs-check-input"${attrs}>` +
					`<span class="docs-box" aria-hidden="true">${checkIcon}</span>`
				);
			},
			blockquote(this: Parser, t: Tokens.Blockquote) {
				const html = this.parser.parse(t.tokens);
				const m = html.match(/^<p>\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]\s*/i);
				if (!m) return `<blockquote>${html}</blockquote>\n`;
				const kind = m[1]!.toLowerCase();
				const c = callouts[kind]!;
				return (
					`<div class="docs-callout docs-callout-${kind}" role="note">${c.icon}` +
					`<div><p class="docs-callout-title">${c.label}</p>${html.replace(m[0], '<p>')}</div></div>\n`
				);
			}
		}
	});
	const html = marked.parse(body, { async: false });
	return { html, headings };
}

/** Search hits: pages and their sections whose text contains every word. */
export type DocHit = { slug: string; page: string; section?: string; id?: string; snippet: string };

const sections = docs.flatMap((d) => {
	const parts = d.markdown.split(/^(?=#{2,3} )/m);
	return parts.map((part, i) => {
		const heading = i === 0 ? undefined : part.match(/^#{2,3} (.+)$/m)?.[1];
		return {
			slug: d.slug,
			page: d.nav,
			section: heading,
			id: heading ? slugify(heading) : undefined,
			text: plain(heading ? part.replace(/^#{2,3} .+$/m, '') : part)
		};
	});
});

export function searchDocs(query: string, limit = 12): DocHit[] {
	const words = query.toLowerCase().split(/\s+/).filter(Boolean);
	if (!words.length) return [];
	const scored: { score: number; hit: DocHit }[] = [];
	for (const s of sections) {
		const title = `${s.page} ${s.section ?? ''}`.toLowerCase();
		const body = s.text.toLowerCase();
		if (!words.every((w) => title.includes(w) || body.includes(w))) continue;
		const score = words.reduce(
			(n, w) => n + (title.includes(w) ? 5 : 0) + (body.includes(w) ? 1 : 0),
			0
		);
		const at = Math.max(0, body.indexOf(words[0]!) - 40);
		const snippet = (at > 0 ? '…' : '') + s.text.slice(at, at + 140).trim() + '…';
		scored.push({
			score,
			hit: { slug: s.slug, page: s.page, section: s.section, id: s.id, snippet }
		});
	}
	scored.sort((a, b) => b.score - a.score);
	return scored.slice(0, limit).map((x) => x.hit);
}
