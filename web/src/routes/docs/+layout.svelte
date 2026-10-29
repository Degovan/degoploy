<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { docBySlug, docGroups } from '#lib/docs.js';
	import DocsSearch from '#lib/components/docs-search.svelte';
	import Logo from '#lib/components/logo.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import * as Sheet from '#lib/components/ui/sheet/index.js';
	import { cn } from '#lib/utils.js';
	import ArrowUpRightIcon from '@lucide/svelte/icons/arrow-up-right';
	import CircleUserIcon from '@lucide/svelte/icons/circle-user';
	import FolderGit2Icon from '@lucide/svelte/icons/folder-git-2';
	import MenuIcon from '@lucide/svelte/icons/menu';
	import RocketIcon from '@lucide/svelte/icons/rocket';
	import SearchIcon from '@lucide/svelte/icons/search';
	import Settings2Icon from '@lucide/svelte/icons/settings-2';
	import UsersIcon from '@lucide/svelte/icons/users';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();

	const icons = {
		rocket: RocketIcon,
		folder: FolderGit2Icon,
		users: UsersIcon,
		settings: Settings2Icon,
		user: CircleUserIcon
	} as Record<string, typeof RocketIcon>;

	const current = $derived(page.params.page ?? '');
	const href = (slug: string) =>
		slug ? resolve('/docs/[[page]]', { page: slug }) : resolve('/docs/[[page]]', {});

	let searchOpen = $state(false);
	let menuOpen = $state(false);
	const modKey =
		typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.userAgent) ? '⌘' : 'Ctrl';

	function onKeydown(e: KeyboardEvent) {
		const el = e.target as HTMLElement;
		const typing = el.closest('input, textarea, select, [contenteditable="true"]');
		if ((e.key === 'k' && (e.metaKey || e.ctrlKey)) || (e.key === '/' && !typing)) {
			e.preventDefault();
			searchOpen = true;
		}
	}
</script>

<svelte:window onkeydown={onKeydown} />

{#snippet nav()}
	{#each docGroups as g (g.title)}
		{@const Icon = icons[g.icon] ?? RocketIcon}
		<div class="flex flex-col gap-0.5">
			<p class="flex items-center gap-2 px-2.5 pb-1.5 text-[13px] font-semibold">
				<Icon class="size-3.5 text-muted-foreground/80" />{g.title}
			</p>
			{#each g.pages as slug (slug)}
				{@const d = docBySlug(slug)}
				{#if d}
					<a
						href={href(slug)}
						onclick={() => (menuOpen = false)}
						class={cn(
							'flex h-8 items-center gap-2 rounded-md pr-2.5 pl-8 text-sm transition-colors',
							slug === current
								? 'bg-muted font-medium text-foreground'
								: 'text-muted-foreground hover:text-foreground'
						)}
						aria-current={slug === current ? 'page' : undefined}
					>
						<span class="min-w-0 flex-1 truncate">{d.nav}</span>
						{#if d.admin}
							<span
								class="rounded bg-[#52A8FF]/12 px-1.5 text-[11px] leading-[18px] font-medium text-[#52A8FF]"
								>Admin</span
							>
						{/if}
					</a>
				{/if}
			{/each}
		</div>
	{/each}
{/snippet}

<div class="flex min-h-svh flex-col">
	<header
		class="sticky top-0 z-40 flex h-16 shrink-0 items-center gap-3 border-b bg-background/90 px-4 backdrop-blur md:px-6"
	>
		<button
			type="button"
			class="-ml-1 flex size-9 items-center justify-center rounded-md text-muted-foreground hover:text-foreground lg:hidden"
			aria-label="Open the guide menu"
			onclick={() => (menuOpen = true)}
		>
			<MenuIcon class="size-5" />
		</button>
		<a href={href('')} class="flex shrink-0 items-center gap-2.5 lg:w-[256px]">
			<Logo class="size-[26px]" />
			<span class="text-sm font-medium">degoploy</span>
			<span
				class="rounded-[5px] border border-input bg-muted px-[7px] text-xs leading-[18px] font-medium text-muted-foreground"
				>Docs</span
			>
		</a>
		<button
			type="button"
			class="hidden h-9 w-full max-w-[360px] items-center gap-2.5 rounded-lg border bg-card pr-1.5 pl-3 text-sm text-muted-foreground/80 transition-colors hover:border-input md:flex"
			onclick={() => (searchOpen = true)}
		>
			<SearchIcon class="size-[15px]" />
			<span class="flex-1 text-left">Search documentation…</span>
			<kbd
				class="rounded-[5px] border border-input bg-muted px-1.5 text-xs leading-5 font-medium text-muted-foreground"
				>{modKey === '⌘' ? '⌘K' : 'Ctrl K'}</kbd
			>
		</button>
		<div class="ml-auto flex items-center gap-2 md:gap-6">
			<button
				type="button"
				class="flex size-9 items-center justify-center rounded-md text-muted-foreground hover:text-foreground md:hidden"
				aria-label="Search documentation"
				onclick={() => (searchOpen = true)}
			>
				<SearchIcon class="size-[18px]" />
			</button>
			<nav class="hidden items-center gap-6 text-sm lg:flex" aria-label="Documentation sections">
				<a href={href('')} class="text-foreground">Guides</a>
				<a
					href="https://github.com/Degovan/degoploy/blob/main/docs/INSTALL.md"
					target="_blank"
					rel="noreferrer"
					class="text-muted-foreground hover:text-foreground">Installation</a
				>
			</nav>
			<Button href={resolve('/(app)/projects')} variant="outline" class="h-8 px-3 text-[13px]">
				<ArrowUpRightIcon class="text-muted-foreground" />
				<span class="hidden sm:inline">Open dashboard</span>
				<span class="sm:hidden">Dashboard</span>
			</Button>
		</div>
	</header>

	<div class="flex flex-1">
		<aside class="hidden w-[280px] shrink-0 border-r lg:block">
			<nav
				class="sticky top-16 flex max-h-[calc(100svh-4rem)] [scrollbar-width:thin] flex-col gap-7 overflow-y-auto py-8 pr-4 pl-6"
				aria-label="Guide pages"
			>
				{@render nav()}
			</nav>
		</aside>
		<main class="relative min-w-0 flex-1 overflow-x-clip">
			{@render children()}
		</main>
	</div>
</div>

<Sheet.Root bind:open={menuOpen}>
	<Sheet.Content side="left" class="w-[300px] overflow-y-auto p-0">
		<Sheet.Header class="border-b px-5 py-4">
			<Sheet.Title class="text-sm">User guide</Sheet.Title>
		</Sheet.Header>
		<nav class="flex flex-col gap-6 px-3 py-5" aria-label="Guide pages">
			{@render nav()}
		</nav>
	</Sheet.Content>
</Sheet.Root>

<DocsSearch bind:open={searchOpen} />
