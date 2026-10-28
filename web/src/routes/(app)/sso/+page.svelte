<script lang="ts">
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { beforeNavigate, replaceState } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { toast } from 'svelte-sonner';
	import { ApiError, errorMessage } from '#lib/api/client.js';
	import { ssoApi, ssoKeys } from '#lib/api/sso.js';
	import type { Role, SSOStatus, UpdateSSORequest } from '#lib/api/types.gen.js';
	import { copyText } from '#lib/clipboard.js';
	import PageHeader from '#lib/components/page-header.svelte';
	import { roleInfo, roles } from '#lib/components/role-picker.svelte';
	import SettingsCard from '#lib/components/settings-card.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import { Switch } from '#lib/components/ui/switch/index.js';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import * as Select from '#lib/components/ui/select/index.js';
	import { formatDateTime, timeAgo } from '#lib/time.js';
	import { cn } from '#lib/utils.js';
	import ArrowRightIcon from '@lucide/svelte/icons/arrow-right';
	import BanIcon from '@lucide/svelte/icons/ban';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import CircleCheckIcon from '@lucide/svelte/icons/circle-check';
	import CircleXIcon from '@lucide/svelte/icons/circle-x';
	import CopyIcon from '@lucide/svelte/icons/copy';
	import FingerprintIcon from '@lucide/svelte/icons/fingerprint-pattern';
	import HistoryIcon from '@lucide/svelte/icons/history';
	import KeyRoundIcon from '@lucide/svelte/icons/key-round';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';
	import LockIcon from '@lucide/svelte/icons/lock';
	import PlugZapIcon from '@lucide/svelte/icons/plug-zap';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
	import UsersIcon from '@lucide/svelte/icons/users';
	import UsersRoundIcon from '@lucide/svelte/icons/users-round';
	import XIcon from '@lucide/svelte/icons/x';

	const queryClient = useQueryClient();
	const status = createQuery(() => ({ queryKey: ssoKeys.status, queryFn: ssoApi.status }));

	// The editable copy of the settings. Role mapping is split into listed
	// groups and the "*" fallback ('' = don't allow anyone else).
	type Mapping = { id: number; group: string; role: Role };
	type Draft = {
		local_login: boolean;
		enabled: boolean;
		provider_name: string;
		issuer: string;
		client_id: string;
		client_secret: string;
		redirect_url: string;
		scopes: string[];
		groups_claim: string;
		mappings: Mapping[];
		fallback: Role | '';
		sync_role_on_login: boolean;
	};
	type Field = keyof Draft;
	type Card = 'password' | 'oidc' | 'mapping';
	const cards: Record<Card, Field[]> = {
		password: ['local_login'],
		oidc: [
			'enabled',
			'provider_name',
			'issuer',
			'client_id',
			'client_secret',
			'redirect_url',
			'scopes'
		],
		mapping: ['groups_claim', 'mappings', 'fallback', 'sync_role_on_login']
	};
	// Server field names ("oidc.issuer") → draft fields.
	const serverField: Record<string, Field> = {
		local_login: 'local_login',
		'oidc.enabled': 'enabled',
		'oidc.provider_name': 'provider_name',
		'oidc.issuer': 'issuer',
		'oidc.client_id': 'client_id',
		'oidc.client_secret': 'client_secret',
		'oidc.redirect_url': 'redirect_url',
		'oidc.scopes': 'scopes',
		'oidc.groups_claim': 'groups_claim',
		'oidc.role_mapping': 'mappings',
		'oidc.sync_role_on_login': 'sync_role_on_login'
	};

	let nextId = 0;
	function toDraft(s: SSOStatus): Draft {
		const mappings = Object.entries(s.role_mapping)
			.filter(([g]) => g !== '*')
			.sort(([ga, ra], [gb, rb]) => rank(ra) - rank(rb) || ga.localeCompare(gb))
			.map(([group, role]) => ({ id: nextId++, group, role: role as Role }));
		return {
			local_login: s.local_login,
			enabled: s.enabled,
			provider_name: s.provider_name,
			issuer: s.issuer,
			client_id: s.client_id,
			client_secret: '',
			redirect_url: s.redirect_url,
			scopes: [...s.scopes],
			groups_claim: s.groups_claim,
			mappings,
			fallback: (s.role_mapping['*'] as Role | undefined) ?? '',
			sync_role_on_login: s.sync_role_on_login
		};
	}
	const rank = (r: string) => roles.findIndex((x) => x.value === r);

	let draft = $state<Draft | null>(null);
	let seed = $state<Draft | null>(null);
	$effect(() => {
		if (status.data && !seed) {
			seed = toDraft(status.data);
			draft = structuredClone($state.snapshot(seed));
		}
	});

	// Mapping ids are local; compare the content only.
	const comparable = (d: Draft, f: Field) =>
		f === 'mappings'
			? JSON.stringify(d.mappings.map((m) => [m.group, m.role]))
			: JSON.stringify(d[f]);
	const dirty = (card: Card) =>
		!!draft && !!seed && cards[card].some((f) => comparable(draft!, f) !== comparable(seed!, f));

	let errors = $state<Partial<Record<Field, string>>>({});
	let failed = $state<Partial<Record<Card, string>>>({});
	let saving = $state<Card | null>(null);

	function request(card: Card, d: Draft): UpdateSSORequest {
		switch (card) {
			case 'password':
				return { local_login: d.local_login };
			case 'oidc':
				return {
					enabled: d.enabled,
					provider_name: d.provider_name,
					issuer: d.issuer,
					client_id: locked('oidc.client_id') ? undefined : d.client_id,
					client_secret: d.client_secret || undefined,
					redirect_url: d.redirect_url,
					scopes: d.scopes
				};
			case 'mapping': {
				const role_mapping: Record<string, string> = {};
				for (const m of d.mappings) if (m.group.trim()) role_mapping[m.group.trim()] = m.role;
				if (d.fallback) role_mapping['*'] = d.fallback;
				return {
					groups_claim: d.groups_claim,
					role_mapping,
					sync_role_on_login: d.sync_role_on_login
				};
			}
		}
	}

	async function save(card: Card) {
		if (!draft || !seed) return;
		for (const f of cards[card]) delete errors[f];
		delete failed[card];
		saving = card;
		try {
			const next = await ssoApi.update(request(card, $state.snapshot(draft) as Draft));
			queryClient.setQueryData(ssoKeys.status, next);
			// Refresh only this card, so edits in the others survive.
			const fresh = toDraft(next);
			for (const f of cards[card]) {
				(seed as Record<Field, unknown>)[f] = structuredClone(fresh[f]);
				(draft as Record<Field, unknown>)[f] = structuredClone(fresh[f]);
			}
			queryClient.invalidateQueries({ queryKey: ['auth', 'providers'] });
			toast.success('Sign-in settings saved', {
				description: next.config_path ? `Written to ${next.config_path}` : undefined
			});
		} catch (err) {
			const field = err instanceof ApiError ? err.field && serverField[err.field] : undefined;
			if (field && cards[card].includes(field)) errors[field] = sentence(err as ApiError);
			else
				failed[card] =
					err instanceof ApiError && err.field === 'sso' ? err.message : errorMessage(err);
		} finally {
			saving = null;
		}
	}
	const sentence = (e: ApiError) => {
		const m = `${e.message}.`;
		return /^(is|must)\b/.test(m) ? `This ${m}` : m.charAt(0).toUpperCase() + m.slice(1);
	};

	beforeNavigate((nav) => {
		const unsaved = (['password', 'oidc', 'mapping'] as Card[]).some(dirty);
		if (unsaved && !confirm('You have unsaved changes. Leave this page?')) nav.cancel();
	});

	// Settings set by the server's environment cannot be edited here.
	const locked = (key: string) => status.data?.locked[key];

	// Discovery: checked for the saved issuer on load, and on demand for the draft.
	let discoverIssuer = $state('');
	$effect(() => {
		if (status.data && !discoverIssuer) discoverIssuer = status.data.issuer;
	});
	const discovery = createQuery(() => ({
		queryKey: ssoKeys.discovery(discoverIssuer),
		queryFn: () => ssoApi.discover(discoverIssuer),
		enabled: !!discoverIssuer,
		retry: false,
		staleTime: 60_000
	}));
	function rediscover() {
		if (!draft?.issuer.trim()) return;
		const issuer = draft.issuer.trim();
		if (issuer === discoverIssuer) discovery.refetch();
		else discoverIssuer = issuer;
	}
	const relative = (url: string) => {
		try {
			const u = new URL(url);
			return discoverIssuer && url.startsWith(new URL(discoverIssuer).origin) ? u.pathname : url;
		} catch {
			return url;
		}
	};

	const connection = $derived.by(() => {
		const s = status.data;
		if (!s?.enabled) return { dot: 'bg-zinc-500', label: 'Off' };
		if (discovery.isError) return { dot: 'bg-red-500', label: 'Unreachable' };
		if (discovery.isSuccess && discoverIssuer === s.issuer)
			return { dot: 'bg-emerald-400', label: 'Connected' };
		return { dot: 'bg-amber-400', label: 'Checking…' };
	});

	// Scopes: openid is always requested.
	let addingScope = $state(false);
	let newScope = $state('');
	function addScope() {
		const s = newScope.trim();
		if (draft && s && !draft.scopes.includes(s)) draft.scopes.push(s);
		newScope = '';
		addingScope = false;
	}

	const suggestedRedirect = $derived(
		typeof location === 'undefined' ? '' : `${location.origin}/auth/oidc/callback`
	);
	async function copy(text: string) {
		if (await copyText(text)) toast.success('Copied');
		else toast.error('Could not copy to the clipboard');
	}

	// The result of a test sign-in comes back as ?test=…&ok=1.
	let test = $state<{ ok: boolean; message: string } | null>(null);
	$effect(() => {
		const msg = page.url.searchParams.get('test');
		if (msg === null) return;
		test = { ok: page.url.searchParams.get('ok') === '1', message: msg };
		replaceState(resolve('/(app)/sso'), {});
		queryClient.invalidateQueries({ queryKey: ssoKeys.status });
	});

	// Section navigation, as on the project settings page.
	const sections = $derived([
		{ id: 'password', label: 'Password' },
		{ id: 'oidc', label: status.data ? `${status.data.provider_name || 'SSO'} OIDC` : 'OIDC' },
		{ id: 'role-mapping', label: 'Role mapping' },
		{ id: 'recent', label: 'Recent sign-ins' }
	]);
	let active = $state('password');
	function updateActive() {
		const ids = sections.map((s) => s.id);
		const root = document.documentElement;
		if (window.scrollY > 0 && window.innerHeight + window.scrollY >= root.scrollHeight - 2) {
			active = ids[ids.length - 1]!;
			return;
		}
		let current = ids[0]!;
		for (const id of ids) {
			const top = document.getElementById(id)?.getBoundingClientRect().top;
			if (top !== undefined && top <= 160) current = id;
		}
		active = current;
	}

	const input = 'h-10 bg-background px-3 dark:bg-background text-sm';
	const mono = 'font-mono text-[13px] md:text-[13px]';
	const label = 'text-[13px] leading-[17px] text-muted-foreground';
	const hint = 'text-xs/normal text-muted-foreground/80';
</script>

<svelte:window onscroll={updateActive} onresize={updateActive} />

{#snippet fieldError(f: Field)}
	{#if errors[f]}<p class="text-xs/normal text-destructive">{errors[f]}</p>{/if}
{/snippet}

{#snippet lockedHint(key: string)}
	<p class={cn(hint, 'flex items-center gap-1.5')}>
		<LockIcon class="size-3" />
		Set by <span class="font-mono">{locked(key)}</span> on the server. Change it there and restart.
	</p>
{/snippet}

{#snippet stat(Icon: typeof UsersIcon, text: string)}
	<span class="flex items-center gap-2 text-[13px] text-muted-foreground">
		<Icon class="size-3.5 text-muted-foreground/80" />{text}
	</span>
{/snippet}

<PageHeader title="Sign-in methods" description="Choose how people sign in to degoploy.">
	{#snippet aside()}
		{#if status.data}
			{@const s = status.data}
			<div class="flex flex-wrap items-center gap-x-5 gap-y-1 text-[13px] text-muted-foreground">
				<span class="flex items-center gap-2">
					<span class={cn('size-2 rounded-full', s.local_login ? 'bg-emerald-400' : 'bg-zinc-500')}
					></span>Password {s.local_login ? 'on' : 'off'}
				</span>
				<span class="flex items-center gap-2">
					<span class={cn('size-2 rounded-full', connection.dot)}></span>{s.provider_name || 'SSO'}
					{connection.label === 'Connected' ? 'connected' : connection.label.toLowerCase()}
				</span>
			</div>
		{/if}
	{/snippet}
</PageHeader>

<div class="container-page grid gap-8 pt-8 pb-24 lg:grid-cols-[13.75rem_minmax(0,1fr)] lg:gap-12">
	<nav
		class="-mx-4 flex gap-0.5 overflow-x-auto px-4 lg:sticky lg:top-32 lg:mx-0 lg:flex-col lg:self-start lg:px-0"
		aria-label="Settings sections"
	>
		{#each sections as s (s.id)}
			<a
				href={`#${s.id}`}
				class={cn(
					'shrink-0 rounded-md px-3 py-2 text-sm whitespace-nowrap transition-colors',
					active === s.id
						? 'bg-muted font-medium text-foreground'
						: 'text-muted-foreground hover:text-foreground'
				)}
				aria-current={active === s.id ? 'location' : undefined}>{s.label}</a
			>
		{/each}
	</nav>

	{#if status.isError}
		<Alert.Root variant="destructive">
			<CircleAlertIcon />
			<Alert.Title>{status.error.message}</Alert.Title>
		</Alert.Root>
	{:else if !draft || !status.data}
		<div class="flex flex-col gap-6">
			<Skeleton class="h-48 rounded-xl" />
			<Skeleton class="h-96 rounded-xl" />
		</div>
	{:else}
		{@const s = status.data}
		{@const d = draft}
		<div class="flex min-w-0 flex-col gap-6">
			{#if !s.config_path}
				<Alert.Root>
					<CircleAlertIcon />
					<Alert.Title>Settings cannot be saved</Alert.Title>
					<Alert.Description>
						degoploy was started without a config file. Start it with
						<code class="font-mono">--config /path/to/degoploy.yaml</code> to save changes here.
					</Alert.Description>
				</Alert.Root>
			{/if}
			{#if test}
				<div
					class={cn(
						'flex items-start gap-3 rounded-lg border px-4 py-3 text-sm',
						test.ok
							? 'border-emerald-400/30 bg-emerald-400/[0.06]'
							: 'border-destructive/40 bg-destructive/10'
					)}
					role="status"
				>
					{#if test.ok}
						<CircleCheckIcon class="mt-0.5 size-4 shrink-0 text-emerald-400" />
					{:else}
						<CircleXIcon class="mt-0.5 size-4 shrink-0 text-destructive" />
					{/if}
					<div class="flex min-w-0 flex-1 flex-col gap-0.5">
						<span class="font-medium"
							>{test.ok ? 'Test sign-in worked' : 'Test sign-in failed'}</span
						>
						<span class="text-muted-foreground">{test.message}</span>
					</div>
					<button
						type="button"
						class="rounded-sm text-muted-foreground hover:text-foreground"
						aria-label="Dismiss"
						onclick={() => (test = null)}><XIcon class="size-4" /></button
					>
				</div>
			{/if}

			<section id="password" class="scroll-mt-32" aria-label="Password">
				<SettingsCard
					title="Password"
					icon={KeyRoundIcon}
					description="Local accounts created on the Users page sign in with their username and password."
					note="Passwords are set and reset from the Users page."
					dirty={dirty('password')}
					saving={saving === 'password'}
					error={failed.password}
					onsave={() => save('password')}
				>
					{#snippet aside()}
						<Switch
							bind:checked={d.local_login}
							disabled={!!locked('local_login')}
							aria-label="Password sign-in"
							class="mt-1 data-[size=default]:h-5 data-[size=default]:w-[34px]"
						/>
					{/snippet}
					<div class="flex flex-wrap gap-x-6 gap-y-2">
						{@render stat(
							UsersIcon,
							`${s.local_users} ${s.local_users === 1 ? 'user signs' : 'users sign'} in with a password`
						)}
						{@render stat(
							HistoryIcon,
							s.last_local_login
								? `Last password sign-in ${timeAgo(s.last_local_login)}`
								: 'No password sign-ins yet'
						)}
					</div>
					{#if locked('local_login')}{@render lockedHint('local_login')}{/if}
					{#if !d.local_login && s.local_login}
						<p
							class="flex items-start gap-2 rounded-md border border-amber-400/30 bg-amber-400/[0.06] px-3 py-2 text-[13px] text-muted-foreground"
						>
							<CircleAlertIcon class="mt-0.5 size-3.5 shrink-0 text-amber-400" />
							Everyone will have to sign in with {s.provider_name || 'SSO'}, including admins. Keep
							a break-glass admin who can still reach the server.
						</p>
					{/if}
				</SettingsCard>
			</section>

			<section id="oidc" class="scroll-mt-32" aria-label="OpenID Connect">
				<SettingsCard
					title={`${d.provider_name || 'SSO'} OIDC`}
					icon={FingerprintIcon}
					description={`Let people sign in with their ${d.provider_name || 'identity provider'} account through OpenID Connect.`}
					dirty={dirty('oidc')}
					saving={saving === 'oidc'}
					error={failed.oidc}
					onsave={() => save('oidc')}
				>
					{#snippet titleAside()}
						<span class="flex items-center gap-2 text-[13px] text-muted-foreground">
							<span class={cn('size-2 rounded-full', connection.dot)}></span>{connection.label}
						</span>
					{/snippet}
					{#snippet aside()}
						<Switch
							bind:checked={d.enabled}
							disabled={!!locked('oidc.enabled')}
							aria-label="Single sign-on"
							class="mt-1 data-[size=default]:h-5 data-[size=default]:w-[34px]"
						/>
					{/snippet}
					{#snippet note()}
						{s.sso_users}
						{s.sso_users === 1 ? 'user' : 'users'}{#if s.last_sso_login}
							· last sign-in via {s.provider_name} {timeAgo(s.last_sso_login)}{/if}
					{/snippet}
					{#snippet extra()}
						<Button
							variant="outline"
							class="h-8 px-3 text-[13px] aria-disabled:pointer-events-none aria-disabled:opacity-50"
							href="/auth/oidc/login?test=1"
							data-sveltekit-reload
							disabled={!s.enabled || dirty('oidc')}
							title={dirty('oidc')
								? 'Save first'
								: 'Sign in with the saved settings, without leaving your session'}
						>
							<PlugZapIcon />
							Test sign-in
						</Button>
					{/snippet}

					{#if locked('oidc.enabled')}{@render lockedHint('oidc.enabled')}{/if}

					<div class="flex flex-col gap-2">
						<Label for="sso-name" class={label}>Provider name</Label>
						<Input id="sso-name" bind:value={d.provider_name} class={input} />
						<p class={hint}>
							Shown on the sign-in button: <span class="font-medium text-muted-foreground"
								>Continue with {d.provider_name || 'SSO'}</span
							>
						</p>
						{@render fieldError('provider_name')}
					</div>

					<div class="flex flex-col gap-2">
						<Label for="sso-issuer" class={label}>Issuer URL</Label>
						<div
							class="flex h-10 items-center gap-1 rounded-md border border-input bg-background pr-1 pl-3 focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/40"
						>
							<input
								id="sso-issuer"
								bind:value={d.issuer}
								disabled={!!locked('oidc.issuer')}
								placeholder="https://sso.example.com"
								spellcheck={false}
								class={cn(
									mono,
									'h-full min-w-0 flex-1 bg-transparent outline-none disabled:opacity-60'
								)}
								onkeydown={(e) => {
									if (e.key === 'Enter') {
										e.preventDefault();
										rediscover();
									}
								}}
							/>
							<Button
								type="button"
								variant="outline"
								class="h-[30px] gap-[7px] px-2.5 text-xs [&_svg]:size-[13px] [&_svg]:text-muted-foreground"
								onclick={rediscover}
								disabled={!d.issuer.trim() || discovery.isFetching}
							>
								<RefreshCwIcon class={discovery.isFetching ? 'animate-spin' : ''} />
								{discoverIssuer ? 'Re-discover' : 'Discover'}
							</Button>
						</div>
						{#if locked('oidc.issuer')}{@render lockedHint('oidc.issuer')}{/if}
						{@render fieldError('issuer')}
						{#if discoverIssuer}
							<div class="overflow-hidden rounded-lg border bg-background">
								<div
									class="flex flex-wrap items-center gap-x-2 gap-y-1 border-b px-3.5 py-2.5 text-[13px]"
								>
									{#if discovery.isPending || discovery.isFetching}
										<LoaderIcon class="size-3.5 animate-spin text-muted-foreground" />
										<span class="font-medium">Checking the discovery document…</span>
									{:else if discovery.isError}
										<CircleXIcon class="size-3.5 text-destructive" />
										<span class="font-medium">Discovery failed</span>
										<span class="text-xs text-muted-foreground/80">{discovery.error.message}</span>
									{:else}
										<CircleCheckIcon class="size-3.5 text-emerald-400" />
										<span class="font-medium">Discovery document found</span>
										<span class="min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground/80"
											>/.well-known/openid-configuration</span
										>
										<span class="text-xs text-muted-foreground/80"
											>checked {timeAgo(discovery.data.checked_at)}</span
										>
									{/if}
								</div>
								{#if discovery.data && !discovery.isError}
									{@const doc = discovery.data}
									{@const claims = doc.claims_supported}
									<dl
										class="grid grid-cols-[6.5rem_minmax(0,1fr)] gap-x-3 gap-y-2 px-3.5 py-3 text-xs"
									>
										{#each [['Authorization', doc.authorization_endpoint], ['Token', doc.token_endpoint], ['Userinfo', doc.userinfo_endpoint], ['JWKS', doc.jwks_uri]] as [k, v] (k)}
											<dt class="text-muted-foreground/80">{k}</dt>
											<dd class="truncate font-mono text-muted-foreground" title={v}>
												{v ? relative(v) : '—'}
											</dd>
										{/each}
										<dt class="text-muted-foreground/80">Claims</dt>
										<dd class="font-mono text-muted-foreground">
											{#if claims.length}
												{[d.groups_claim || 'groups', 'email', 'name']
													.map((c) => `${c} ${claims.includes(c) ? '✓' : '✗'}`)
													.join(' · ')}
											{:else}
												not listed by the provider
											{/if}
										</dd>
									</dl>
								{/if}
							</div>
						{/if}
					</div>

					<div class="grid gap-4 md:grid-cols-2">
						<div class="flex min-w-0 flex-col gap-2">
							<Label for="sso-client" class={label}>Client ID</Label>
							<Input
								id="sso-client"
								bind:value={d.client_id}
								disabled={!!locked('oidc.client_id')}
								spellcheck={false}
								class={cn(input, mono)}
							/>
							{#if locked('oidc.client_id')}
								{@render lockedHint('oidc.client_id')}
							{:else}
								<p class={hint}>From your {d.provider_name || 'provider'} application.</p>
							{/if}
							{@render fieldError('client_id')}
						</div>
						<div class="flex min-w-0 flex-col gap-2">
							<Label for="sso-secret" class={label}>Client secret</Label>
							{#if locked('oidc.client_secret')}
								<div
									class="flex h-10 items-center gap-1.5 rounded-md border border-input bg-muted px-3"
								>
									<LockIcon class="size-3.5 shrink-0 text-muted-foreground/80" />
									<span class="min-w-0 flex-1 truncate font-mono text-[13px] text-muted-foreground"
										>{locked('oidc.client_secret')?.startsWith('$')
											? locked('oidc.client_secret')
											: `$${locked('oidc.client_secret')}`}</span
									>
									<span class="flex items-center gap-2 text-xs text-muted-foreground">
										<span
											class={cn(
												'size-2 rounded-full',
												s.client_secret_set ? 'bg-emerald-400' : 'bg-red-500'
											)}
										></span>{s.client_secret_set ? 'Set' : 'Not set'}
									</span>
								</div>
								<p class={hint}>Read from the server environment. Change it there and restart.</p>
							{:else}
								<Input
									id="sso-secret"
									type="password"
									bind:value={d.client_secret}
									autocomplete="off"
									placeholder={s.client_secret_set
										? '•••••••• saved · type to replace'
										: 'Paste the client secret'}
									class={cn(input, mono)}
								/>
								<p class={hint}>Saved encrypted with the server key, never in plain text.</p>
							{/if}
							{@render fieldError('client_secret')}
						</div>
					</div>

					<div class="flex flex-col gap-2">
						<Label for="sso-redirect" class={label}>Redirect URL</Label>
						<div
							class="flex h-10 items-center gap-1 rounded-md border border-input bg-background pr-1 pl-3 focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/40"
						>
							<input
								id="sso-redirect"
								bind:value={d.redirect_url}
								disabled={!!locked('oidc.redirect_url')}
								placeholder={suggestedRedirect}
								spellcheck={false}
								class={cn(
									mono,
									'h-full min-w-0 flex-1 bg-transparent outline-none disabled:opacity-60'
								)}
							/>
							<button
								type="button"
								class="flex size-[30px] shrink-0 items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground [&_svg]:size-[15px]"
								aria-label="Copy redirect URL"
								onclick={() => copy(d.redirect_url || suggestedRedirect)}><CopyIcon /></button
							>
						</div>
						{#if locked('oidc.redirect_url')}
							{@render lockedHint('oidc.redirect_url')}
						{:else if !d.redirect_url}
							<p class={hint}>
								For this address:
								<button
									type="button"
									class="font-mono text-muted-foreground underline-offset-2 hover:text-foreground hover:underline"
									onclick={() => (d.redirect_url = suggestedRedirect)}>{suggestedRedirect}</button
								>
							</p>
						{:else}
							<p class={hint}>
								Add this to the Redirect URLs of your {d.provider_name || 'provider'} application.
							</p>
						{/if}
						{@render fieldError('redirect_url')}
					</div>

					<div class="flex flex-col gap-2">
						<span class={label}>Scopes</span>
						<div class="flex flex-wrap items-center gap-2">
							{#each d.scopes as scope (scope)}
								<span
									class="inline-flex h-7 items-center gap-1.5 rounded-md border border-input bg-muted pr-2 pl-2.5 font-mono text-xs"
								>
									{scope}
									{#if scope === 'openid'}
										<LockIcon
											class="size-3 text-muted-foreground/80"
											aria-label="Always requested"
										/>
									{:else}
										<button
											type="button"
											class="rounded-sm text-muted-foreground/80 hover:text-foreground"
											aria-label={`Remove ${scope}`}
											onclick={() => (d.scopes = d.scopes.filter((x) => x !== scope))}
											><XIcon class="size-3" /></button
										>
									{/if}
								</span>
							{/each}
							{#if addingScope}
								<!-- svelte-ignore a11y_autofocus -->
								<input
									bind:value={newScope}
									autofocus
									placeholder="scope"
									spellcheck={false}
									class="h-7 w-28 rounded-md border border-ring bg-background px-2 font-mono text-xs outline-none"
									onkeydown={(e) => {
										if (e.key === 'Enter') {
											e.preventDefault();
											addScope();
										} else if (e.key === 'Escape') {
											addingScope = false;
											newScope = '';
										}
									}}
									onblur={addScope}
								/>
							{:else}
								<button
									type="button"
									class="inline-flex h-7 items-center gap-1.5 rounded-md border border-input px-2.5 text-xs text-muted-foreground hover:text-foreground"
									onclick={() => (addingScope = true)}
								>
									<PlusIcon class="size-3" />Add scope
								</button>
							{/if}
						</div>
						{#if discovery.data?.scopes_supported.length && !discovery.isError}
							<p class={hint}>
								Supported by your server: {discovery.data.scopes_supported.join(', ')}.
							</p>
						{/if}
						{@render fieldError('scopes')}
					</div>
				</SettingsCard>
			</section>

			<section id="role-mapping" class="scroll-mt-32" aria-label="Role mapping">
				<SettingsCard
					title="Role mapping"
					icon={UsersRoundIcon}
					description={`Give people a degoploy role based on their ${d.provider_name || 'provider'} groups.`}
					dirty={dirty('mapping')}
					saving={saving === 'mapping'}
					error={failed.mapping}
					onsave={() => save('mapping')}
				>
					{#snippet note()}
						<a
							href={resolve('/(app)/roles')}
							class="inline-flex items-center gap-1 hover:text-foreground"
							>Compare what each role can do <ArrowRightIcon class="size-3.5" /></a
						>
					{/snippet}

					<div class="flex max-w-[360px] flex-col gap-2">
						<Label for="sso-claim" class={label}>Groups claim</Label>
						<Input
							id="sso-claim"
							bind:value={d.groups_claim}
							spellcheck={false}
							class={cn(input, mono)}
						/>
						<p class={hint}>
							The claim that lists a person's groups. Casdoor sends them as <span
								class="font-mono text-muted-foreground">&lt;organization&gt;/&lt;group&gt;</span
							>.
						</p>
						{@render fieldError('groups_claim')}
					</div>

					<div class="flex flex-col gap-2">
						<div
							class="hidden grid-cols-[minmax(0,1fr)_16px_200px_30px] gap-3 text-[13px] text-muted-foreground md:grid"
						>
							<span>{d.provider_name || 'Provider'} group</span><span></span><span>Role</span><span
							></span>
						</div>
						{#snippet roleSelect(
							value: Role | '',
							onchange: (v: Role | '') => void,
							allowNone: boolean,
							aria: string
						)}
							{@const info = value ? roleInfo(value) : null}
							<Select.Root type="single" {value} onValueChange={(v) => onchange(v as Role | '')}>
								<Select.Trigger
									class="h-10! w-full justify-start bg-background md:w-[200px] [&>svg:last-child]:ml-auto"
									aria-label={aria}
								>
									{#if info}
										<info.icon class="size-3.5 text-muted-foreground" />{info.label}
									{:else}
										<BanIcon class="size-3.5 text-muted-foreground" />Don't allow
									{/if}
								</Select.Trigger>
								<Select.Content>
									{#each roles as r (r.value)}
										<Select.Item value={r.value} label={r.label}>
											<r.icon class="size-3.5 text-muted-foreground" />{r.label}
										</Select.Item>
									{/each}
									{#if allowNone}
										<Select.Item value="" label="Don't allow">
											<BanIcon class="size-3.5 text-muted-foreground" />Don't allow
										</Select.Item>
									{/if}
								</Select.Content>
							</Select.Root>
						{/snippet}
						{#each d.mappings as m, i (m.id)}
							<div
								class="grid grid-cols-[minmax(0,1fr)_30px] items-center gap-x-3 gap-y-2 md:grid-cols-[minmax(0,1fr)_16px_200px_30px]"
							>
								<Input
									bind:value={m.group}
									placeholder="organization/group"
									spellcheck={false}
									aria-label="Group"
									class={cn(input, mono)}
								/>
								<ArrowRightIcon class="hidden size-4 text-muted-foreground/80 md:block" />
								<div class="col-start-1 row-start-2 md:col-start-auto md:row-start-auto">
									{@render roleSelect(
										m.role,
										(v) => (m.role = v as Role),
										false,
										`Role for ${m.group}`
									)}
								</div>
								<button
									type="button"
									class="col-start-2 row-start-1 flex size-[30px] items-center justify-center rounded-md text-muted-foreground/80 hover:bg-accent hover:text-foreground md:col-start-auto md:row-start-auto [&_svg]:size-[15px]"
									aria-label={`Remove ${m.group || 'mapping'}`}
									onclick={() => d.mappings.splice(i, 1)}><Trash2Icon /></button
								>
							</div>
						{/each}
						<div
							class="grid grid-cols-[minmax(0,1fr)_30px] items-center gap-x-3 gap-y-2 md:grid-cols-[minmax(0,1fr)_16px_200px_30px]"
						>
							<div
								class="flex h-10 items-center gap-2.5 rounded-md border border-input bg-muted px-3 text-sm text-muted-foreground"
							>
								<span class="rounded bg-accent px-[7px] font-mono text-[13px] text-foreground"
									>*</span
								>
								Everyone else
							</div>
							<ArrowRightIcon class="hidden size-4 text-muted-foreground/80 md:block" />
							<div class="col-start-1 row-start-2 md:col-start-auto md:row-start-auto">
								{@render roleSelect(
									d.fallback,
									(v) => (d.fallback = v),
									true,
									'Role for everyone else'
								)}
							</div>
						</div>
						<div class="flex flex-wrap items-center gap-x-4 gap-y-2 pt-1">
							<Button
								type="button"
								variant="outline"
								class="h-8 px-2.5 text-[13px]"
								onclick={() => d.mappings.push({ id: nextId++, group: '', role: 'viewer' })}
							>
								<PlusIcon />Add mapping
							</Button>
							<p class={hint}>
								Set Everyone else to “Don't allow” to reject people outside these groups. The
								highest role among a person's groups applies.
							</p>
						</div>
						{@render fieldError('mappings')}
					</div>

					<label class="flex cursor-pointer items-start gap-3 border-t pt-4">
						<Switch
							bind:checked={d.sync_role_on_login}
							class="mt-0.5 data-[size=default]:h-5 data-[size=default]:w-[34px]"
						/>
						<span class="flex flex-col gap-1">
							<span class="text-sm font-medium">Sync role on every sign-in</span>
							<span class={hint}>
								The role follows {d.provider_name || 'provider'} groups. Changes made on the Users page
								are overwritten the next time that person signs in.
							</span>
						</span>
					</label>
				</SettingsCard>
			</section>

			<section
				id="recent"
				class="scroll-mt-32 overflow-hidden rounded-xl border bg-card"
				aria-labelledby="recent-title"
			>
				<div class="flex flex-col gap-1.5 border-b p-6">
					<h3 id="recent-title" class="text-xl font-semibold tracking-[-0.4px]">
						Recent SSO sign-ins
					</h3>
					<p class="text-sm text-muted-foreground">
						The last 20 attempts and tests since degoploy started, with the groups the provider
						sent. Use them to find the exact group names for the mapping.
					</p>
				</div>
				{#if s.recent_logins.length === 0}
					<p class="px-6 py-8 text-center text-sm text-muted-foreground">No SSO sign-ins yet.</p>
				{:else}
					<ul>
						{#each s.recent_logins as l, i (i)}
							{@const good = l.outcome.startsWith('signed in') || l.outcome === 'test: allowed'}
							<li
								class="flex flex-col gap-2 border-b px-6 py-3.5 last:border-b-0"
								data-testid="sso-login"
							>
								<div class="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm">
									<span class={cn('size-2 rounded-full', good ? 'bg-emerald-400' : 'bg-red-500')}
									></span>
									<span class="font-medium">{l.username || l.email || l.subject}</span>
									<span class="text-muted-foreground">{l.outcome}</span>
									{#if l.role}
										<span class="text-muted-foreground">as {roleInfo(l.role as Role).label}</span>
									{/if}
									<time
										class="ml-auto text-xs text-muted-foreground/80"
										title={formatDateTime(l.at)}>{timeAgo(l.at)}</time
									>
								</div>
								<div class="flex flex-wrap gap-1.5">
									{#each l.groups as g (g)}
										<button
											type="button"
											class="rounded-md border border-input bg-muted px-2 py-0.5 font-mono text-xs hover:border-muted-foreground/60"
											title="Copy group name"
											onclick={() => copy(g)}>{g}</button
										>
									{:else}
										<span class={hint}>No groups in the <code>{s.groups_claim}</code> claim.</span>
									{/each}
								</div>
								<details class="text-xs">
									<summary class="cursor-pointer text-muted-foreground/80">Claims</summary>
									<pre
										class="mt-2 overflow-x-auto rounded-md border bg-background p-3 font-mono">{JSON.stringify(
											l.claims,
											null,
											2
										)}</pre>
								</details>
							</li>
						{/each}
					</ul>
				{/if}
			</section>
		</div>
	{/if}
</div>
