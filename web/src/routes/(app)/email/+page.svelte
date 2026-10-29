<script lang="ts">
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { beforeNavigate } from '$app/navigation';
	import { toast } from 'svelte-sonner';
	import { ApiError, errorMessage } from '#lib/api/client.js';
	import { emailApi, emailKeys } from '#lib/api/email.js';
	import type { EmailStatus, UpdateEmailRequest } from '#lib/api/types.gen.js';
	import PageHeader from '#lib/components/page-header.svelte';
	import SettingsCard from '#lib/components/settings-card.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import { session } from '#lib/stores/session.svelte.js';
	import { formatDateTime, timeAgo } from '#lib/time.js';
	import { cn } from '#lib/utils.js';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import CircleCheckIcon from '@lucide/svelte/icons/circle-check';
	import CircleXIcon from '@lucide/svelte/icons/circle-x';
	import InfoIcon from '@lucide/svelte/icons/info';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';
	import LockIcon from '@lucide/svelte/icons/lock';
	import MailCheckIcon from '@lucide/svelte/icons/mail-check';
	import MailXIcon from '@lucide/svelte/icons/mail-x';
	import PencilIcon from '@lucide/svelte/icons/pencil';
	import SendIcon from '@lucide/svelte/icons/send';
	import ServerIcon from '@lucide/svelte/icons/server';

	const queryClient = useQueryClient();
	const status = createQuery(() => ({ queryKey: emailKeys.status, queryFn: emailApi.status }));

	type Encryption = EmailStatus['encryption'];
	type Draft = {
		host: string;
		port: string;
		encryption: Encryption;
		username: string;
		password: string;
		from_name: string;
		from_address: string;
	};
	type Field = keyof Draft;
	const toDraft = (s: EmailStatus): Draft => ({
		host: s.host,
		port: String(s.port || ''),
		encryption: s.encryption,
		username: s.username,
		password: '',
		from_name: s.from_name,
		from_address: s.from_address
	});

	let draft = $state<Draft | null>(null);
	let seed = $state<Draft | null>(null);
	$effect(() => {
		if (status.data && !seed) {
			seed = toDraft(status.data);
			draft = { ...seed };
		}
	});
	const dirty = $derived(
		!!draft && !!seed && (Object.keys(seed) as Field[]).some((k) => draft![k] !== seed![k])
	);

	let replacingPassword = $state(false);
	let errors = $state<Partial<Record<Field, string>>>({});
	let failed = $state('');
	let saving = $state(false);

	const locked = (key: string) => status.data?.locked[`smtp.${key}`];

	async function save() {
		if (!draft || !seed) return;
		errors = {};
		failed = '';
		const port = Number(draft.port);
		if (draft.port && !Number.isInteger(port)) {
			errors = { port: 'Enter a port number.' };
			return;
		}
		const body: UpdateEmailRequest = {};
		for (const k of ['host', 'encryption', 'username', 'from_name', 'from_address'] as const) {
			if (draft[k] !== seed[k]) (body as Record<string, unknown>)[k] = draft[k];
		}
		if (draft.port !== seed.port) body.port = port;
		if (draft.password) body.password = draft.password;
		saving = true;
		try {
			const next = await emailApi.update(body);
			queryClient.setQueryData(emailKeys.status, next);
			seed = toDraft(next);
			draft = { ...seed };
			replacingPassword = false;
			toast.success('Email settings saved', {
				description: next.configured ? 'Send a test email to enable invites.' : undefined
			});
		} catch (err) {
			const field = err instanceof ApiError && err.field?.replace(/^smtp\./, '');
			if (field && field in draft) {
				const m = (err as ApiError).message;
				errors = {
					[field]: /^(is|must)\b/.test(m)
						? `This ${m}.`
						: `${m.charAt(0).toUpperCase()}${m.slice(1)}.`
				};
			} else failed = errorMessage(err);
		} finally {
			saving = false;
		}
	}

	beforeNavigate((nav) => {
		if (dirty && !confirm('You have unsaved changes. Leave this page?')) nav.cancel();
	});

	// Common ports follow the encryption choice unless a custom one was typed.
	const defaultPorts: Record<Encryption, string> = { none: '25', starttls: '587', tls: '465' };
	function setEncryption(e: Encryption) {
		if (!draft) return;
		if (Object.values(defaultPorts).includes(draft.port) || !draft.port)
			draft.port = defaultPorts[e];
		draft.encryption = e;
	}
	const encryptions: { value: Encryption; label: string }[] = [
		{ value: 'none', label: 'None' },
		{ value: 'starttls', label: 'STARTTLS' },
		{ value: 'tls', label: 'SSL/TLS' }
	];
	let segButtons = $state<HTMLButtonElement[]>([]);
	function onSegKeydown(e: KeyboardEvent) {
		const step = { ArrowRight: 1, ArrowLeft: -1, ArrowDown: 1, ArrowUp: -1 }[e.key];
		if (!step || !draft) return;
		e.preventDefault();
		const i = encryptions.findIndex((x) => x.value === draft!.encryption);
		const next = (i + step + encryptions.length) % encryptions.length;
		setEncryption(encryptions[next]!.value);
		segButtons[next]?.focus();
	}

	// Test email
	let testTo = $state('');
	$effect(() => {
		if (!testTo && session.user?.email) testTo = session.user.email;
	});
	let testing = $state(false);
	let testError = $state('');
	async function sendTest(e: SubmitEvent) {
		e.preventDefault();
		testError = '';
		testing = true;
		try {
			const next = await emailApi.test(testTo.trim());
			queryClient.setQueryData(emailKeys.status, next);
			if (next.last_test?.ok) toast.success(`Test email delivered to ${next.last_test.to}`);
			else toast.error('The test email was not delivered');
		} catch (err) {
			testError = errorMessage(err);
		} finally {
			testing = false;
		}
	}

	const health = $derived.by(() => {
		const s = status.data;
		if (!s?.configured)
			return { dot: 'bg-zinc-500', label: 'Not set', summary: 'Not set up · invites off' };
		if (s.ready)
			return { dot: 'bg-emerald-400', label: 'Verified', summary: 'Verified · invites enabled' };
		if (s.last_test?.current && !s.last_test.ok)
			return { dot: 'bg-red-500', label: 'Test failed', summary: 'Test failed · invites paused' };
		return { dot: 'bg-amber-400', label: 'Not tested', summary: 'Not tested · invites paused' };
	});

	const sections = [
		{ id: 'smtp', label: 'SMTP server' },
		{ id: 'test', label: 'Test email' }
	];
	let active = $state('smtp');
	function updateActive() {
		const root = document.documentElement;
		if (window.scrollY > 0 && window.innerHeight + window.scrollY >= root.scrollHeight - 2) {
			active = 'test';
			return;
		}
		const top = document.getElementById('test')?.getBoundingClientRect().top;
		active = top !== undefined && top <= 160 ? 'test' : 'smtp';
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
		<LockIcon class="size-3" />Set by <span class="font-mono">{locked(key)}</span> on the server.
	</p>
{/snippet}

<PageHeader title="Email" description="The SMTP server degoploy uses to send invitations.">
	{#snippet aside()}
		{#if status.data}
			<span class="flex items-center gap-2 text-[13px] text-muted-foreground">
				<span class={cn('size-2 rounded-full', health.dot)}></span>{health.summary}
			</span>
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
			<Skeleton class="h-96 rounded-xl" />
			<Skeleton class="h-48 rounded-xl" />
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

			<section id="smtp" class="scroll-mt-32" aria-label="SMTP server">
				<SettingsCard
					title="SMTP server"
					icon={ServerIcon}
					description="Outgoing mail server for invitation emails."
					{dirty}
					{saving}
					error={failed}
					onsave={save}
				>
					{#snippet titleAside()}
						<span class="flex items-center gap-2 text-[13px] text-muted-foreground">
							<span class={cn('size-2 rounded-full', health.dot)}></span>{health.label}
						</span>
					{/snippet}
					{#snippet note()}
						<span class="flex items-center gap-2">
							<InfoIcon class="size-3.5 shrink-0 text-muted-foreground/80" />
							Saving changes pauses invites until a new test email is delivered.
						</span>
					{/snippet}

					<div class="grid gap-4 sm:grid-cols-[minmax(0,1fr)_7.5rem]">
						<div class="flex min-w-0 flex-col gap-2">
							<Label for="smtp-host" class={label}>Host</Label>
							<Input
								id="smtp-host"
								bind:value={d.host}
								disabled={!!locked('host')}
								placeholder="smtp.example.com"
								spellcheck={false}
								autocomplete="off"
								class={cn(input, mono)}
							/>
							{#if locked('host')}{@render lockedHint('host')}{/if}
							{@render fieldError('host')}
						</div>
						<div class="flex min-w-0 flex-col gap-2">
							<Label for="smtp-port" class={label}>Port</Label>
							<Input
								id="smtp-port"
								bind:value={d.port}
								disabled={!!locked('port')}
								inputmode="numeric"
								class={cn(input, mono)}
							/>
							{#if locked('port')}{@render lockedHint('port')}{/if}
							{@render fieldError('port')}
						</div>
					</div>

					<div class="flex flex-col gap-2">
						<span id="smtp-encryption" class={label}>Encryption</span>
						<div
							class="grid h-10 max-w-[400px] grid-cols-3 gap-0.5 rounded-md border border-input p-[3px]"
							role="radiogroup"
							aria-labelledby="smtp-encryption"
						>
							{#each encryptions as e, i (e.value)}
								{@const on = d.encryption === e.value}
								<button
									bind:this={segButtons[i]}
									type="button"
									role="radio"
									aria-checked={on}
									tabindex={on ? 0 : -1}
									class={cn(
										'rounded-sm text-[13px] font-medium transition-colors outline-none focus-visible:ring-3 focus-visible:ring-ring/50',
										on ? 'bg-accent text-foreground' : 'text-muted-foreground hover:text-foreground'
									)}
									onclick={() => setEncryption(e.value)}
									onkeydown={onSegKeydown}>{e.label}</button
								>
							{/each}
						</div>
						<p class={hint}>
							Usually STARTTLS on port 587, or SSL/TLS on port 465. None sends the password
							unencrypted.
						</p>
						{@render fieldError('encryption')}
					</div>

					<div class="grid gap-4 md:grid-cols-2">
						<div class="flex min-w-0 flex-col gap-2">
							<Label for="smtp-user" class={label}>Username</Label>
							<Input
								id="smtp-user"
								bind:value={d.username}
								disabled={!!locked('username')}
								placeholder="Leave empty for no login"
								spellcheck={false}
								autocomplete="off"
								class={cn(input, mono)}
							/>
							{#if locked('username')}{@render lockedHint('username')}{/if}
							{@render fieldError('username')}
						</div>
						<div class="flex min-w-0 flex-col gap-2">
							<Label for="smtp-password" class={label}>Password</Label>
							{#if locked('password')}
								<div
									class="flex h-10 items-center gap-1.5 rounded-md border border-input bg-muted px-3"
								>
									<LockIcon class="size-3.5 text-muted-foreground/80" />
									<span class="flex-1 truncate font-mono text-[13px] text-muted-foreground"
										>{locked('password')}</span
									>
									<span class="text-xs text-muted-foreground"
										>{s.password_set ? 'Set' : 'Not set'}</span
									>
								</div>
							{:else if s.password_set && !replacingPassword}
								<div
									class="flex h-10 items-center gap-1 rounded-md border border-input bg-background pr-1 pl-3"
								>
									<span
										class="flex-1 font-mono text-[13px] tracking-widest text-muted-foreground"
										aria-label="Saved password">••••••••••••</span
									>
									<Button
										type="button"
										variant="outline"
										class="h-[30px] gap-1.5 px-2.5 text-xs [&_svg]:size-3"
										onclick={() => (replacingPassword = true)}
									>
										<PencilIcon />Replace
									</Button>
								</div>
							{:else}
								<Input
									id="smtp-password"
									type="password"
									bind:value={d.password}
									autocomplete="new-password"
									autofocus={replacingPassword}
									placeholder={s.password_set ? 'New password' : ''}
									class={cn(input, mono)}
								/>
							{/if}
							<p class={hint}>Saved encrypted with the server key.</p>
							{@render fieldError('password')}
						</div>
					</div>

					<div class="h-px bg-border"></div>

					<div class="grid gap-4 md:grid-cols-2">
						<div class="flex min-w-0 flex-col gap-2">
							<Label for="smtp-from-name" class={label}>From name</Label>
							<Input
								id="smtp-from-name"
								bind:value={d.from_name}
								disabled={!!locked('from_name')}
								class={input}
							/>
							{#if locked('from_name')}{@render lockedHint('from_name')}{/if}
							{@render fieldError('from_name')}
						</div>
						<div class="flex min-w-0 flex-col gap-2">
							<Label for="smtp-from" class={label}>From address</Label>
							<Input
								id="smtp-from"
								type="email"
								bind:value={d.from_address}
								disabled={!!locked('from_address')}
								placeholder="no-reply@example.com"
								spellcheck={false}
								class={cn(input, mono)}
							/>
							{#if locked('from_address')}{@render lockedHint('from_address')}{/if}
							{@render fieldError('from_address')}
						</div>
					</div>
					{#if d.from_address}
						<p class={hint}>
							Invitations arrive from <span class="font-medium text-muted-foreground"
								>{d.from_name ? `${d.from_name} <${d.from_address}>` : d.from_address}</span
							>
						</p>
					{/if}
				</SettingsCard>
			</section>

			<section
				id="test"
				class="scroll-mt-32 overflow-hidden rounded-xl border bg-card"
				aria-labelledby="test-title"
			>
				<div class="flex flex-col gap-5 p-6">
					<div class="flex items-start gap-4">
						<span
							class="flex size-10 shrink-0 items-center justify-center rounded-lg border border-input bg-muted"
						>
							<SendIcon class="size-[18px]" />
						</span>
						<div class="flex flex-col gap-1.5">
							<h3 id="test-title" class="text-xl font-semibold tracking-[-0.4px]">
								Send a test email
							</h3>
							<p class="text-sm/normal text-muted-foreground">
								Inviting people is enabled only after a test email is delivered with the current
								settings.
							</p>
						</div>
					</div>

					<div class="flex flex-col gap-4 sm:pl-14">
						<form class="flex flex-col gap-2" onsubmit={sendTest} novalidate>
							<Label for="test-to" class={label}>Send to</Label>
							<div class="flex flex-col gap-2 sm:flex-row">
								<Input
									id="test-to"
									type="email"
									bind:value={testTo}
									placeholder="you@example.com"
									class={cn(input, 'flex-1')}
								/>
								<Button
									type="submit"
									variant="outline"
									class="h-10 px-3 text-[13px]"
									disabled={testing || !s.configured || dirty || !testTo.trim()}
									title={dirty ? 'Save the settings first' : undefined}
								>
									{#if testing}<LoaderIcon class="animate-spin" />{:else}<SendIcon />{/if}
									Send test email
								</Button>
							</div>
							{#if testError}
								<p class="text-xs/normal text-destructive" role="alert">{testError}</p>
							{:else if !s.configured}
								<p class={hint}>Set the host, port and from address above first.</p>
							{:else if dirty}
								<p class={hint}>Save the settings above first; the test uses the saved ones.</p>
							{/if}
						</form>

						{#if s.last_test}
							{@const t = s.last_test}
							{@const good = t.ok && t.current}
							<div
								class={cn(
									'flex flex-wrap items-center gap-3 rounded-lg border px-3.5 py-3',
									good && 'border-emerald-400/20 bg-emerald-400/[0.06]',
									!t.ok && 'border-destructive/30 bg-destructive/[0.06]',
									t.ok && !t.current && 'bg-background'
								)}
								role="status"
							>
								{#if t.ok}
									<CircleCheckIcon
										class={cn(
											'size-4 shrink-0',
											t.current ? 'text-emerald-400' : 'text-muted-foreground'
										)}
									/>
								{:else}
									<CircleXIcon class="size-4 shrink-0 text-destructive" />
								{/if}
								<div class="flex min-w-0 flex-1 flex-col gap-[3px]">
									<span class="text-[13px] font-medium">
										{t.ok ? 'Delivered to' : 'Not delivered to'}
										{t.to}{t.current ? '' : ' with earlier settings'}
									</span>
									<span
										class="font-mono text-xs break-words text-muted-foreground/80"
										title={formatDateTime(t.at)}
									>
										{t.detail} · {s.host}:{s.port} · {timeAgo(t.at)}
									</span>
								</div>
								<span
									class="inline-flex h-[22px] shrink-0 items-center gap-[5px] rounded-[5px] border border-input bg-muted px-[7px] text-xs"
								>
									{#if s.ready}
										<MailCheckIcon class="size-3 text-emerald-400" />Invites enabled
									{:else}
										<MailXIcon class="size-3 text-amber-400" />Invites paused
									{/if}
								</span>
							</div>
						{/if}
					</div>
				</div>
			</section>
		</div>
	{/if}
</div>
