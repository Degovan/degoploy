<script lang="ts">
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import * as Sidebar from '#lib/components/ui/sidebar/index.js';
	import { session } from '#lib/stores/session.svelte.js';
	import type { Action } from '#lib/api/types.gen.js';
	import Logo from './logo.svelte';
	import NavUser from './nav-user.svelte';
	import FolderGitIcon from '@lucide/svelte/icons/folder-git-2';
	import UsersIcon from '@lucide/svelte/icons/users';
	import ScrollTextIcon from '@lucide/svelte/icons/scroll-text';
	import KeyRoundIcon from '@lucide/svelte/icons/key-round';
	import type { Component } from 'svelte';

	// hrefs are resolved from literal routes: resolve() cannot take a union of route ids.
	type NavItem = { title: string; href: string; icon: Component; requires?: Action };

	const main: NavItem[] = [
		{ title: 'Projects', href: resolve('/(app)/projects'), icon: FolderGitIcon }
	];
	const admin: NavItem[] = [
		{ title: 'Users', href: resolve('/(app)/users'), icon: UsersIcon, requires: 'user.manage' },
		{
			title: 'Audit log',
			href: resolve('/(app)/audit'),
			icon: ScrollTextIcon,
			requires: 'audit.view'
		},
		{ title: 'SSO', href: resolve('/(app)/sso'), icon: KeyRoundIcon, requires: 'user.manage' }
	];

	const visibleAdmin = $derived(admin.filter((i) => !i.requires || session.can(i.requires)));

	function isActive(href: string) {
		return page.url.pathname === href || page.url.pathname.startsWith(href + '/');
	}
</script>

{#snippet navGroup(label: string, items: NavItem[])}
	<Sidebar.Group>
		<Sidebar.GroupLabel>{label}</Sidebar.GroupLabel>
		<Sidebar.Menu>
			{#each items as item (item.href)}
				<Sidebar.MenuItem>
					<Sidebar.MenuButton isActive={isActive(item.href)} tooltipContent={item.title}>
						{#snippet child({ props })}
							<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- resolved in NavItem -->
							<a href={item.href} {...props}>
								<item.icon />
								<span>{item.title}</span>
							</a>
						{/snippet}
					</Sidebar.MenuButton>
				</Sidebar.MenuItem>
			{/each}
		</Sidebar.Menu>
	</Sidebar.Group>
{/snippet}

<Sidebar.Root collapsible="icon">
	<Sidebar.Header>
		<Sidebar.Menu>
			<Sidebar.MenuItem>
				<Sidebar.MenuButton size="lg">
					{#snippet child({ props })}
						<a href={resolve('/(app)/projects')} {...props}>
							<Logo class="size-8" />
							<div class="grid flex-1 text-left leading-tight">
								<span class="font-semibold">degoploy</span>
								<span class="text-xs text-muted-foreground">Webhook deployer</span>
							</div>
						</a>
					{/snippet}
				</Sidebar.MenuButton>
			</Sidebar.MenuItem>
		</Sidebar.Menu>
	</Sidebar.Header>

	<Sidebar.Content>
		{@render navGroup('Deploy', main)}
		{#if visibleAdmin.length}
			{@render navGroup('Administration', visibleAdmin)}
		{/if}
	</Sidebar.Content>

	<Sidebar.Footer>
		<NavUser />
	</Sidebar.Footer>
	<Sidebar.Rail />
</Sidebar.Root>
