<script lang="ts">
	import { goto } from '$app/navigation';
	import { searchDocs, type DocHit } from '#lib/docs.js';
	import * as Dialog from '#lib/components/ui/dialog/index.js';
	import { cn } from '#lib/utils.js';
	import CornerDownLeftIcon from '@lucide/svelte/icons/corner-down-left';
	import FileTextIcon from '@lucide/svelte/icons/file-text';
	import HashIcon from '@lucide/svelte/icons/hash';
	import SearchIcon from '@lucide/svelte/icons/search';

	// Searches every page and section of the guide; opens with ⌘K / Ctrl+K or "/".
	let { open = $bindable(false) }: { open?: boolean } = $props();

	let query = $state('');
	let active = $state(0);
	let input = $state<HTMLInputElement | null>(null);
	const hits = $derived(searchDocs(query));

	$effect(() => {
		if (open) {
			query = '';
			active = 0;
		}
	});
	$effect(() => {
		void hits;
		active = 0;
	});

	const hrefOf = (h: DocHit) => `/docs${h.slug ? `/${h.slug}` : ''}${h.id ? `#${h.id}` : ''}`;

	function choose(h: DocHit | undefined) {
		if (!h) return;
		open = false;
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- built from known guide slugs
		goto(hrefOf(h));
	}

	function onkeydown(e: KeyboardEvent) {
		if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
			e.preventDefault();
			const n = hits.length;
			if (n) active = (active + (e.key === 'ArrowDown' ? 1 : n - 1)) % n;
			document.getElementById(`docs-hit-${active}`)?.scrollIntoView({ block: 'nearest' });
		} else if (e.key === 'Enter') {
			e.preventDefault();
			choose(hits[active]);
		}
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Content
		showCloseButton={false}
		class="top-[15vh] flex max-h-[70vh] translate-y-0 flex-col gap-0 overflow-hidden rounded-xl border border-input bg-card p-0 shadow-[0_24px_64px_rgb(0_0_0/0.8)] ring-0 sm:max-w-[560px]"
		onOpenAutoFocus={(e) => {
			e.preventDefault();
			input?.focus();
		}}
	>
		<Dialog.Title class="sr-only">Search documentation</Dialog.Title>
		<div class="flex items-center gap-3 border-b px-4">
			<SearchIcon class="size-4 shrink-0 text-muted-foreground/80" />
			<input
				bind:this={input}
				bind:value={query}
				type="search"
				placeholder="Search documentation…"
				aria-label="Search documentation"
				aria-controls="docs-hits"
				aria-activedescendant={hits.length ? `docs-hit-${active}` : undefined}
				class="h-12 min-w-0 flex-1 bg-transparent text-[15px] outline-none placeholder:text-muted-foreground/80"
				{onkeydown}
			/>
			<kbd class="rounded border border-input bg-muted px-1.5 text-[11px] text-muted-foreground"
				>Esc</kbd
			>
		</div>
		<div class="min-h-0 flex-1 [scrollbar-width:thin] overflow-y-auto p-2">
			{#if !query.trim()}
				<p class="px-3 py-8 text-center text-sm text-muted-foreground/80">
					Search pages and sections of the user guide.
				</p>
			{:else if hits.length === 0}
				<p class="px-3 py-8 text-center text-sm text-muted-foreground/80">
					No results for “{query.trim()}”.
				</p>
			{:else}
				<ul id="docs-hits" role="listbox" aria-label="Results">
					{#each hits as h, i (hrefOf(h))}
						<li
							id={`docs-hit-${i}`}
							role="option"
							aria-selected={i === active}
							class={cn(
								'flex cursor-pointer items-start gap-3 rounded-lg px-3 py-2.5',
								i === active && 'bg-muted'
							)}
							onmousemove={() => (active = i)}
							onclick={() => choose(h)}
							onkeydown={() => {}}
						>
							{#if h.section}
								<HashIcon class="mt-0.5 size-4 shrink-0 text-muted-foreground/80" />
							{:else}
								<FileTextIcon class="mt-0.5 size-4 shrink-0 text-muted-foreground/80" />
							{/if}
							<span class="flex min-w-0 flex-1 flex-col gap-0.5">
								<span class="truncate text-sm">
									{#if h.section}<span class="text-muted-foreground">{h.page} ›</span>
										{h.section}{:else}{h.page}{/if}
								</span>
								<span class="line-clamp-2 text-xs text-muted-foreground/80">{h.snippet}</span>
							</span>
							{#if i === active}
								<CornerDownLeftIcon class="mt-0.5 size-3.5 shrink-0 text-muted-foreground/80" />
							{/if}
						</li>
					{/each}
				</ul>
			{/if}
		</div>
	</Dialog.Content>
</Dialog.Root>
