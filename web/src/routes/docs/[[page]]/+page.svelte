<script lang="ts">
	import { resolve } from '$app/paths';
	import { mode } from 'mode-watcher';
	import { toast } from 'svelte-sonner';
	import { copyText } from '#lib/clipboard.js';
	import { docBySlug, docOrder, editUrl, renderDoc } from '#lib/docs.js';
	import { diagramConfig } from '#lib/docs-diagrams.js';
	import { cn } from '#lib/utils.js';
	import ArrowLeftIcon from '@lucide/svelte/icons/arrow-left';
	import ArrowRightIcon from '@lucide/svelte/icons/arrow-right';
	import ArrowUpIcon from '@lucide/svelte/icons/arrow-up';
	import BookOpenIcon from '@lucide/svelte/icons/book-open';
	import CalendarIcon from '@lucide/svelte/icons/calendar';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import CopyIcon from '@lucide/svelte/icons/copy';
	import PencilIcon from '@lucide/svelte/icons/pencil';
	import ShieldCheckIcon from '@lucide/svelte/icons/shield-check';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const meta = $derived(docBySlug(data.slug)!);
	const doc = $derived(renderDoc(data.slug));
	const index = $derived(docOrder.indexOf(data.slug));
	const prev = $derived(index > 0 ? docBySlug(docOrder[index - 1]!) : undefined);
	const next = $derived(index < docOrder.length - 1 ? docBySlug(docOrder[index + 1]!) : undefined);

	const href = (slug: string) =>
		slug ? resolve('/docs/[[page]]', { page: slug }) : resolve('/docs/[[page]]', {});
	const updated = $derived(
		meta.updated
			? new Intl.DateTimeFormat('en-GB', { dateStyle: 'medium' }).format(new Date(meta.updated))
			: ''
	);

	let article = $state<HTMLElement | null>(null);

	// Diagrams: Mermaid is large, so it loads only here, and redraws when the theme changes.
	$effect(() => {
		const dark = mode.current !== 'light';
		void doc.html;
		const nodes = [...(article?.querySelectorAll<HTMLElement>('.docs-diagram') ?? [])];
		if (!nodes.length) return;
		let stale = false;
		import('mermaid').then(async ({ default: mermaid }) => {
			mermaid.initialize(diagramConfig(dark));
			for (const [i, node] of nodes.entries()) {
				if (stale) return;
				try {
					const { svg } = await mermaid.render(`docs-diagram-${i}`, node.dataset.mermaid ?? '');
					if (!stale) node.innerHTML = svg;
				} catch {
					node.textContent = node.dataset.mermaid ?? '';
					node.classList.add('docs-diagram-failed');
				}
			}
		});
		return () => {
			stale = true;
		};
	});

	// Checklist ticks are remembered per page in this browser.
	const checksKey = $derived(`degoploy.docs.checks.${data.slug || 'index'}`);
	const checkboxes = () => [
		...(article?.querySelectorAll<HTMLInputElement>('.docs-check-input:not(:disabled)') ?? [])
	];
	$effect(() => {
		void doc.html;
		let saved: number[] = [];
		try {
			saved = JSON.parse(localStorage.getItem(checksKey) ?? '[]');
		} catch {
			// Storage unavailable or corrupt: start unticked.
		}
		checkboxes().forEach((box, i) => (box.checked = saved.includes(i)));
	});
	function onArticleChange(e: Event) {
		if (!(e.target as HTMLElement).matches('.docs-check-input')) return;
		const ticked = checkboxes().flatMap((box, i) => (box.checked ? [i] : []));
		try {
			localStorage.setItem(checksKey, JSON.stringify(ticked));
		} catch {
			// Storage unavailable: ticks last until the page is left.
		}
	}

	// Copy buttons on code blocks (rendered HTML, so the listener is delegated).
	$effect(() => {
		const el = article;
		if (!el) return;
		el.addEventListener('click', onArticleClick);
		el.addEventListener('change', onArticleChange);
		return () => {
			el.removeEventListener('click', onArticleClick);
			el.removeEventListener('change', onArticleChange);
		};
	});
	function onArticleClick(e: MouseEvent) {
		const button = (e.target as HTMLElement).closest<HTMLButtonElement>('[data-copy]');
		const code = button?.closest('.docs-code')?.querySelector('code')?.textContent;
		if (!button || code == null) return;
		copyText(code).then((ok) => {
			if (!ok) return toast.error('Could not copy to the clipboard');
			button.textContent = 'Copied';
			setTimeout(() => (button.textContent = 'Copy'), 1500);
		});
	}

	async function copyMarkdown() {
		if (await copyText(meta.markdown)) toast.success('Page copied as Markdown');
		else toast.error('Could not copy to the clipboard');
	}

	// "On this page": the last heading scrolled past the header is current.
	let active = $state('');
	function updateActive() {
		const root = document.documentElement;
		if (window.scrollY > 0 && window.innerHeight + window.scrollY >= root.scrollHeight - 2) {
			active = doc.headings.at(-1)?.id ?? '';
			return;
		}
		let current = '';
		for (const h of doc.headings) {
			const top = document.getElementById(h.id)?.getBoundingClientRect().top;
			if (top !== undefined && top <= 120) current = h.id;
		}
		active = current;
	}

	// New page: jump to the heading in the URL, or to the top.
	$effect(() => {
		void doc.html;
		const id = decodeURIComponent(location.hash.slice(1));
		if (id) document.getElementById(id)?.scrollIntoView();
		else window.scrollTo(0, 0);
		updateActive();
	});
</script>

<svelte:window onscroll={updateActive} onresize={updateActive} />

<svelte:head>
	<title>{meta.nav} · degoploy docs</title>
	<meta name="description" content={meta.lead} />
</svelte:head>

<!-- A soft light at the top of the page, as in the design. -->
<div
	class="pointer-events-none absolute inset-x-0 top-0 h-[520px] bg-[radial-gradient(60%_60%_at_40%_0%,color-mix(in_oklab,var(--color-foreground)_6%,transparent),transparent)]"
	aria-hidden="true"
></div>

<div class="relative flex justify-center gap-16 px-5 pt-10 pb-24 md:px-10 lg:px-16 lg:pt-12">
	<article class="w-full max-w-[720px] min-w-0">
		<header class="flex flex-col gap-4 pb-8">
			<nav class="flex items-center gap-1.5 text-[13px]" aria-label="Breadcrumb">
				<span class="text-muted-foreground/80">{meta.group}</span>
				<ChevronRightIcon class="size-[13px] text-muted-foreground/80" />
				<span class="font-medium text-muted-foreground">{meta.nav}</span>
			</nav>
			<h1
				class="text-[34px] leading-[1.1] font-semibold tracking-[-1.2px] md:text-[44px] md:tracking-[-1.6px]"
			>
				{meta.title}
			</h1>
			{#if meta.lead}
				<p class="text-[17px] leading-[1.6] text-muted-foreground md:text-[19px]">{meta.lead}</p>
			{/if}
			<div
				class="flex flex-wrap items-center gap-x-4 gap-y-2 pt-1 text-[13px] text-muted-foreground/80"
			>
				{#if meta.admin}
					<span
						class="inline-flex h-[22px] items-center gap-[5px] rounded-[5px] border border-input bg-muted px-[7px] text-xs font-medium text-foreground"
					>
						<ShieldCheckIcon class="size-3" />Admin only
					</span>
				{/if}
				<span class="flex items-center gap-1.5">
					<BookOpenIcon class="size-3.5" />{meta.minutes} min read
				</span>
				{#if updated}
					<span class="flex items-center gap-1.5">
						<CalendarIcon class="size-3.5" />Updated {updated}
					</span>
				{/if}
			</div>
		</header>

		<!-- The guide is bundled from docs/guide at build time; it is not user input. -->
		<div bind:this={article} class="docs-prose">
			<!-- eslint-disable-next-line svelte/no-at-html-tags -->
			{@html doc.html}
		</div>

		<footer class="flex flex-col gap-7 pt-16">
			<div
				class="flex flex-wrap items-center justify-between gap-3 border-b pb-7 text-sm text-muted-foreground"
			>
				<span>Something wrong or missing on this page?</span>
				<!-- eslint-disable svelte/no-navigation-without-resolve -- external link -->
				<a
					href={editUrl(data.slug)}
					target="_blank"
					rel="noreferrer"
					class="flex items-center gap-1.5 text-[13px] text-muted-foreground/80 hover:text-foreground"
				>
					<PencilIcon class="size-[13px]" />Suggest an edit
				</a>
				<!-- eslint-enable svelte/no-navigation-without-resolve -->
			</div>
			<nav class="grid gap-4 sm:grid-cols-2" aria-label="Previous and next page">
				{#if prev}
					<a
						href={href(prev.slug)}
						class="flex flex-col gap-1.5 rounded-xl border px-5 py-4 transition-colors hover:border-input hover:bg-card"
					>
						<span class="flex items-center gap-1.5 text-[13px] text-muted-foreground/80">
							<ArrowLeftIcon class="size-3.5" />Previous
						</span>
						<span class="text-base font-medium">{prev.nav}</span>
					</a>
				{:else}
					<span class="hidden sm:block"></span>
				{/if}
				{#if next}
					<a
						href={href(next.slug)}
						class="flex flex-col items-end gap-1.5 rounded-xl border px-5 py-4 text-right transition-colors hover:border-input hover:bg-card"
					>
						<span class="flex items-center gap-1.5 text-[13px] text-muted-foreground/80">
							Next<ArrowRightIcon class="size-3.5" />
						</span>
						<span class="text-base font-medium">{next.nav}</span>
					</a>
				{/if}
			</nav>
		</footer>
	</article>

	<aside class="hidden w-[220px] shrink-0 xl:block">
		<div class="sticky top-28 flex flex-col gap-3.5 pt-2">
			{#if doc.headings.length}
				<p class="font-mono text-[11px] tracking-[0.8px] text-muted-foreground/80 uppercase">
					On this page
				</p>
				<nav class="flex flex-col border-l" aria-label="On this page">
					{#each doc.headings as h (h.id)}
						<a
							href={`#${h.id}`}
							class={cn(
								'-ml-px border-l py-1.5 text-[13px] leading-[1.4] transition-colors',
								h.depth === 3 ? 'pl-7' : 'pl-3.5',
								active === h.id
									? 'border-foreground font-medium text-foreground'
									: 'border-transparent text-muted-foreground/80 hover:text-foreground'
							)}
							aria-current={active === h.id ? 'location' : undefined}>{h.text}</a
						>
					{/each}
				</nav>
			{/if}
			<div class="mt-2 flex flex-col gap-1 border-t pt-4 text-[13px] text-muted-foreground/80">
				<button
					type="button"
					class="flex items-center gap-2 py-1 text-left hover:text-foreground"
					onclick={() => window.scrollTo({ top: 0, behavior: 'smooth' })}
				>
					<ArrowUpIcon class="size-[13px]" />Back to top
				</button>
				<button
					type="button"
					class="flex items-center gap-2 py-1 text-left hover:text-foreground"
					onclick={copyMarkdown}
				>
					<CopyIcon class="size-[13px]" />Copy page as Markdown
				</button>
			</div>
		</div>
	</aside>
</div>

<style>
	.docs-prose {
		font-size: 16px;
		line-height: 1.7;
		color: var(--color-muted-foreground);
	}
	.docs-prose :global(> * + *) {
		margin-top: 1rem;
	}
	.docs-prose :global(h2) {
		display: flex;
		align-items: baseline;
		gap: 0.6rem;
		margin-top: 3.5rem;
		color: var(--color-foreground);
		font-size: 26px;
		font-weight: 600;
		letter-spacing: -0.6px;
		line-height: 1.3;
	}
	.docs-prose :global(h2:first-child) {
		margin-top: 0.5rem;
	}
	.docs-prose :global(h3) {
		margin-top: 2rem;
		color: var(--color-foreground);
		font-size: 18px;
		font-weight: 600;
		letter-spacing: -0.2px;
		line-height: 1.4;
	}
	.docs-prose :global(:is(h2, h3)) {
		scroll-margin-top: 6rem;
	}
	.docs-prose :global(.docs-anchor) {
		color: var(--color-input);
		font-size: 22px;
		font-weight: 500;
		text-decoration: none;
	}
	.docs-prose :global(.docs-anchor:hover) {
		color: var(--color-muted-foreground);
	}
	.docs-prose :global(a:not(.docs-anchor)) {
		color: var(--color-foreground);
		text-decoration: underline;
		text-decoration-color: color-mix(in oklab, var(--color-foreground) 35%, transparent);
		text-underline-offset: 3px;
	}
	.docs-prose :global(a:not(.docs-anchor):hover) {
		text-decoration-color: var(--color-foreground);
	}
	.docs-prose :global(strong) {
		color: var(--color-foreground);
		font-weight: 600;
	}
	.docs-prose :global(ul:not(.docs-checklist)) {
		list-style: disc;
		padding-left: 1.4rem;
	}
	.docs-prose :global(li + li) {
		margin-top: 0.4rem;
	}
	.docs-prose :global(li::marker) {
		color: var(--color-input);
	}
	.docs-prose :global(code) {
		border: 1px solid var(--color-border);
		border-radius: 5px;
		background: var(--color-card);
		padding: 0.05rem 0.35rem;
		color: var(--color-foreground);
		font-family: var(--font-mono);
		font-size: 0.84em;
	}

	/* Numbered steps: circles joined by a line. */
	.docs-prose :global(ol.docs-steps) {
		counter-reset: step;
		padding: 0.75rem 0 0 0.875rem;
	}
	.docs-prose :global(ol.docs-steps > li) {
		position: relative;
		counter-increment: step;
		margin: 0;
		border-left: 1px solid var(--color-border);
		padding: 0.125rem 0 1.75rem 2.125rem;
	}
	.docs-prose :global(ol.docs-steps > li:last-child) {
		border-left-color: transparent;
		padding-bottom: 0;
	}
	.docs-prose :global(ol.docs-steps > li::before) {
		content: counter(step);
		position: absolute;
		top: 0;
		left: -14px;
		display: flex;
		align-items: center;
		justify-content: center;
		width: 28px;
		height: 28px;
		border: 1px solid var(--color-input);
		border-radius: 50%;
		background: var(--color-muted);
		color: var(--color-foreground);
		font-family: var(--font-mono);
		font-size: 13px;
	}
	.docs-prose :global(ol.docs-steps > li > * + *) {
		margin-top: 0.75rem;
	}

	/* Checklists: a real checkbox drawn as a rounded box. */
	.docs-prose :global(ul.docs-checklist) {
		display: flex;
		flex-direction: column;
		gap: 0.375rem;
	}
	.docs-prose :global(li.docs-check) {
		margin: 0;
	}
	.docs-prose :global(li.docs-check > label) {
		display: flex;
		align-items: flex-start;
		gap: 0.75rem;
		cursor: pointer;
		border-radius: 8px;
		padding: 0.25rem 0.375rem;
		margin: 0 -0.375rem;
		color: var(--color-foreground);
		transition: background-color 0.15s;
	}
	.docs-prose :global(li.docs-check > label:hover) {
		background: var(--color-card);
	}
	.docs-prose :global(li.docs-check.is-fact > label) {
		cursor: default;
	}
	.docs-prose :global(li.docs-check.is-fact > label:hover) {
		background: none;
	}
	.docs-prose :global(.docs-check-input) {
		position: absolute;
		width: 1px;
		height: 1px;
		opacity: 0;
	}
	.docs-prose :global(.docs-box) {
		display: flex;
		flex-shrink: 0;
		align-items: center;
		justify-content: center;
		width: 20px;
		height: 20px;
		margin-top: 0.2rem;
		border: 1px solid color-mix(in oklab, var(--color-foreground) 22%, transparent);
		border-radius: 6px;
		background: var(--color-muted);
		color: transparent;
		transition:
			background-color 0.15s,
			border-color 0.15s,
			color 0.15s;
	}
	.docs-prose :global(.docs-check-input:checked + .docs-box) {
		border-color: var(--color-foreground);
		background: var(--color-foreground);
		color: var(--color-background);
	}
	.docs-prose :global(.docs-check-input:focus-visible + .docs-box) {
		outline: 2px solid color-mix(in oklab, var(--color-ring) 60%, transparent);
		outline-offset: 2px;
	}
	.docs-prose :global(li.docs-check > label:has(.docs-check-input:checked:not(:disabled))) {
		color: var(--color-muted-foreground);
		text-decoration: line-through;
		text-decoration-color: color-mix(in oklab, var(--color-muted-foreground) 50%, transparent);
	}
	/* Stated facts: a soft green tick. */
	.docs-prose :global(li.is-fact .docs-check-input:checked + .docs-box) {
		border-color: color-mix(in oklab, #50e3c2 35%, transparent);
		background: color-mix(in oklab, #50e3c2 12%, transparent);
		color: #50e3c2;
	}

	/* Callouts. */
	.docs-prose :global(.docs-callout) {
		--tone: #52a8ff;
		display: flex;
		gap: 0.75rem;
		border: 1px solid color-mix(in oklab, var(--tone) 20%, transparent);
		border-radius: 10px;
		background: color-mix(in oklab, var(--tone) 5%, transparent);
		padding: 0.875rem 1rem;
		font-size: 14px;
		line-height: 1.6;
	}
	.docs-prose :global(.docs-callout-tip) {
		--tone: #50e3c2;
	}
	.docs-prose :global(.docs-callout-important) {
		--tone: #a87fff;
	}
	.docs-prose :global(.docs-callout-warning) {
		--tone: #ffb224;
	}
	.docs-prose :global(.docs-callout-caution) {
		--tone: #ff6166;
	}
	.docs-prose :global(.docs-callout > svg) {
		flex-shrink: 0;
		margin-top: 0.15rem;
		color: var(--tone);
	}
	.docs-prose :global(.docs-callout > div > * + *) {
		margin-top: 0.25rem;
	}
	.docs-prose :global(.docs-callout-title) {
		color: var(--color-foreground);
		font-weight: 600;
	}

	/* Code blocks. */
	.docs-prose :global(.docs-code) {
		overflow: hidden;
		border: 1px solid var(--color-border);
		border-radius: 12px;
		background: var(--color-card);
	}
	.docs-prose :global(.docs-code-head) {
		display: flex;
		align-items: center;
		justify-content: space-between;
		border-bottom: 1px solid var(--color-border);
		padding: 0.4rem 0.5rem 0.4rem 1rem;
		font-family: var(--font-mono);
		font-size: 12px;
	}
	.docs-prose :global(.docs-copy) {
		border-radius: 6px;
		padding: 0.2rem 0.6rem;
		color: var(--color-muted-foreground);
		font-family: var(--font-sans);
	}
	.docs-prose :global(.docs-copy:hover) {
		background: var(--color-muted);
		color: var(--color-foreground);
	}
	.docs-prose :global(.docs-code pre) {
		overflow-x: auto;
		padding: 1rem;
		color: var(--color-foreground);
		font-size: 13px;
		line-height: 1.7;
	}
	.docs-prose :global(.docs-code code) {
		border: 0;
		background: none;
		padding: 0;
		font-size: inherit;
	}

	/* Tables. */
	.docs-prose :global(.docs-table) {
		overflow-x: auto;
		border: 1px solid var(--color-border);
		border-radius: 12px;
	}
	.docs-prose :global(table) {
		width: 100%;
		border-collapse: collapse;
		font-size: 14px;
		line-height: 1.55;
	}
	.docs-prose :global(th) {
		background: var(--color-card);
		color: color-mix(in oklab, var(--color-muted-foreground) 80%, transparent);
		font-size: 12px;
		font-weight: 500;
		text-align: left;
	}
	.docs-prose :global(:is(th, td)) {
		border-bottom: 1px solid var(--color-border);
		padding: 0.7rem 1rem;
		vertical-align: top;
	}
	.docs-prose :global(tbody tr:last-child td) {
		border-bottom: 0;
	}
	.docs-prose :global(td:first-child) {
		color: var(--color-foreground);
	}
	.docs-prose :global(tr.docs-row-group td) {
		background: var(--color-background);
		color: color-mix(in oklab, var(--color-muted-foreground) 80%, transparent);
		font-size: 12px;
		font-weight: 500;
	}
	.docs-prose :global(tr.docs-row-group strong) {
		color: inherit;
		font-weight: inherit;
	}

	/* FAQ accordions. */
	.docs-prose :global(details) {
		border-bottom: 1px solid var(--color-border);
		padding: 1.125rem 0;
	}
	.docs-prose :global(details + details) {
		margin-top: 0;
	}
	.docs-prose :global(h2 + details) {
		margin-top: 0.5rem;
	}
	.docs-prose :global(summary) {
		display: flex;
		cursor: pointer;
		list-style: none;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
		color: var(--color-foreground);
		font-size: 16px;
		font-weight: 500;
	}
	.docs-prose :global(summary::-webkit-details-marker) {
		display: none;
	}
	.docs-prose :global(summary::after) {
		content: '+';
		color: var(--color-muted-foreground);
		font-size: 20px;
		font-weight: 300;
		line-height: 1;
	}
	.docs-prose :global(details[open] summary::after) {
		content: '−';
	}
	.docs-prose :global(details > :not(summary)) {
		margin-top: 0.625rem;
		font-size: 15px;
	}

	/* Diagrams. */
	.docs-prose :global(.docs-diagram) {
		display: flex;
		justify-content: safe center;
		overflow-x: auto;
		border: 1px solid var(--color-border);
		border-radius: 14px;
		/* A faint dot grid, on the same canvas color the diagram theme uses. */
		background: radial-gradient(circle, #e4e4e7 1px, transparent 1.2px) 0 0 / 18px 18px #ffffff;
		padding: 2rem 1.5rem;
	}
	:global(.dark) .docs-prose :global(.docs-diagram) {
		background: radial-gradient(circle, #262626 1px, transparent 1.2px) 0 0 / 18px 18px #121212;
	}
	.docs-prose :global(.docs-diagram svg) {
		max-width: 100%;
		height: auto;
	}
	.docs-prose :global(.docs-diagram:empty) {
		min-height: 8rem;
	}
	.docs-prose :global(.docs-diagram-failed) {
		justify-content: flex-start;
		font-family: var(--font-mono);
		font-size: 13px;
		white-space: pre;
	}
</style>
