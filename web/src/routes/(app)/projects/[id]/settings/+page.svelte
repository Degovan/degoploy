<script lang="ts">
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { beforeNavigate, goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import { ApiError, errorMessage } from '#lib/api/client.js';
	import { majorOf, nodeApi, nodeKeys } from '#lib/api/node.js';
	import { projectKeys, projectsApi } from '#lib/api/projects.js';
	import type { Project, Provider, UpdateProjectRequest } from '#lib/api/types.gen.js';
	import DeployKeyCard from '#lib/components/deploy-key-card.svelte';
	import PageHeader from '#lib/components/page-header.svelte';
	import ProviderIcon from '#lib/components/provider-icon.svelte';
	import SettingsCard from '#lib/components/settings-card.svelte';
	import WebhookCard from '#lib/components/webhook-card.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import { Switch } from '#lib/components/ui/switch/index.js';
	import * as AlertDialog from '#lib/components/ui/alert-dialog/index.js';
	import { session } from '#lib/stores/session.svelte.js';
	import { formatDuration } from '#lib/time.js';
	import { cn } from '#lib/utils.js';
	import FolderIcon from '@lucide/svelte/icons/folder';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';
	import LockIcon from '@lucide/svelte/icons/lock';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	const queryClient = useQueryClient();

	const project = createQuery(() => ({
		queryKey: projectKeys.detail(data.id),
		queryFn: () => projectsApi.get(data.id)
	}));
	const canSecrets = $derived(session.can('project.secrets'));
	const secrets = createQuery(() => ({
		queryKey: projectKeys.secrets(data.id),
		queryFn: () => projectsApi.secrets(data.id),
		enabled: canSecrets,
		staleTime: Infinity
	}));

	type Draft = {
		name: string;
		enabled: boolean;
		provider: Provider;
		repo_url: string;
		work_dir: string;
		script_source: Project['script_source'];
		script_path: string;
		auto_pull: boolean;
		timeout_sec: number;
		node_version: string;
	};
	type Field = keyof Draft;

	const fromProject = (p: Project): Draft => ({
		name: p.name,
		enabled: p.enabled,
		provider: p.provider,
		repo_url: p.repo_url,
		work_dir: p.work_dir,
		script_source: p.script_source,
		script_path: p.script_path || '.degoploy/deploy.sh',
		auto_pull: p.auto_pull,
		timeout_sec: p.timeout_sec,
		node_version: p.node_version
	});

	// Each card edits its own fields of the draft and saves only those.
	let saved = $state<Draft | null>(null);
	let draft = $state<Draft | null>(null);
	let errors = $state<Partial<Record<Field, string>>>({});
	// An error that is not about one field, shown on the card that failed to save.
	let failed = $state<{ card: string; message: string } | null>(null);
	let saving = $state<string | null>(null);

	$effect(() => {
		if (project.data && !saved) {
			saved = fromProject(project.data);
			draft = { ...saved };
		}
	});

	const cards = {
		name: ['name'],
		webhooks: ['enabled'],
		repository: ['provider', 'repo_url'],
		workdir: ['work_dir'],
		script: ['script_source', 'script_path', 'auto_pull'],
		node: ['node_version'],
		timeout: ['timeout_sec']
	} satisfies Record<string, Field[]>;
	type Card = keyof typeof cards;

	const dirty = (card: Card) =>
		!!draft &&
		!!saved &&
		(cards[card] as Field[]).some((f) => String(draft![f]) !== String(saved![f]));
	const anyDirty = $derived((Object.keys(cards) as Card[]).some(dirty));

	// Repository scripts only exist after a pull.
	$effect(() => {
		if (draft?.script_source === 'repo') draft.auto_pull = true;
	});

	async function save(card: Card) {
		if (!draft || !saved) return;
		const fields = cards[card] as Field[];
		if (card === 'node' && nodeMode === 'version' && !draft.node_version.trim()) {
			errors = { node_version: 'Enter a version, such as 22 or lts.' };
			return;
		}
		const body: Record<string, unknown> = {};
		for (const f of fields) body[f] = f === 'timeout_sec' ? Number(draft[f]) : draft[f];
		if (card === 'script' && draft.script_source === 'managed') delete body.script_path;
		saving = card;
		errors = {};
		failed = null;
		try {
			const p = await projectsApi.update(data.id, body as UpdateProjectRequest);
			queryClient.setQueryData(projectKeys.detail(p.id), p);
			await queryClient.invalidateQueries({ queryKey: projectKeys.all, exact: true });
			if (card === 'script')
				await queryClient.invalidateQueries({ queryKey: projectKeys.script(p.id) });
			// Take the server's normalized values for this card only; other cards keep their edits.
			const next = fromProject(p);
			for (const f of fields) {
				(saved as Record<Field, unknown>)[f] = next[f];
				(draft as Record<Field, unknown>)[f] = next[f];
			}
			toast.success('Settings saved');
		} catch (err) {
			if (err instanceof ApiError && err.field) errors = { [err.field as Field]: err.message };
			else failed = { card, message: errorMessage(err) };
		} finally {
			saving = null;
		}
	}
	const cardError = (card: Card) => (failed?.card === card ? failed.message : '');

	beforeNavigate((nav) => {
		if (anyDirty && !confirm('Discard unsaved settings?')) nav.cancel();
	});

	async function regenerate(kind: 'key' | 'secret') {
		try {
			const next =
				kind === 'key'
					? await projectsApi.regenerateKey(data.id)
					: await projectsApi.regenerateSecret(data.id);
			queryClient.setQueryData(projectKeys.secrets(data.id), next);
			toast.success(
				kind === 'key'
					? 'New deploy key generated. Replace the old key on your Git host.'
					: 'New webhook secret generated. Update it in your webhook settings.'
			);
		} catch (err) {
			toast.error(errorMessage(err));
		}
	}

	let deleteOpen = $state(false);
	let confirmSlug = $state('');
	let deleting = $state(false);
	async function remove() {
		deleting = true;
		try {
			await projectsApi.remove(data.id);
			saved = draft = null;
			queryClient.removeQueries({ queryKey: projectKeys.detail(data.id) });
			await queryClient.invalidateQueries({ queryKey: projectKeys.all, exact: true });
			toast.success('Project deleted');
			await goto(resolve('/(app)/projects'));
		} catch (err) {
			toast.error(errorMessage(err));
		} finally {
			deleting = false;
		}
	}

	// Section navigation: the link of the section nearest the top is active.
	const sections = $derived([
		{ id: 'general', label: 'General' },
		{ id: 'repository', label: 'Repository' },
		{ id: 'deploy-script', label: 'Deploy Script' },
		...(canSecrets ? [{ id: 'deploy-key', label: 'Deploy Key & Webhook' }] : []),
		{ id: 'danger-zone', label: 'Danger Zone' }
	]);
	let active = $state('general');
	let hashHandled = false;

	// The active section is the last one whose top has passed below the sticky
	// header; at the bottom of the page it is the last section, which may be too
	// short to ever reach the top.
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

	// Once the sections are rendered (the page waits for the project), honour a
	// #section link and sync the active link.
	$effect(() => {
		if (!draft) return;
		const hash = location.hash.slice(1);
		if (!hashHandled && sections.some((s) => s.id === hash)) {
			hashHandled = true;
			document.getElementById(hash)?.scrollIntoView();
		}
		updateActive();
	});

	// Node.js: Auto, a version, or None. The default version is shown to admins.
	const canNode = $derived(session.can('user.manage'));
	const nodeStatus = createQuery(() => ({
		queryKey: nodeKeys.status,
		queryFn: nodeApi.status,
		enabled: canNode
	}));
	// Quick picks: lts and the maintained LTS lines from nodejs.org.
	const nodeReleases = createQuery(() => ({
		queryKey: nodeKeys.releases,
		queryFn: nodeApi.releases,
		enabled: canNode,
		staleTime: 60 * 60 * 1000,
		retry: false
	}));
	const nodePicks = $derived([
		'lts',
		...(nodeReleases.data
			? nodeReleases.data
					.filter((r) => r.lts)
					.slice(0, 3)
					.map((r) => majorOf(r.version))
			: ['24', '22', '20'])
	]);
	type NodeMode = 'auto' | 'version' | 'none';
	const nodeMode = $derived<NodeMode>(
		draft?.node_version === 'auto' ? 'auto' : draft?.node_version === 'none' ? 'none' : 'version'
	);
	let lastSpec = 'lts';
	function setNodeMode(m: NodeMode) {
		if (!draft || m === nodeMode) return;
		if (nodeMode === 'version') lastSpec = draft.node_version || lastSpec;
		draft.node_version = m === 'version' ? lastSpec : m;
	}
	const nodeModes: { value: NodeMode; label: string }[] = [
		{ value: 'auto', label: 'Auto' },
		{ value: 'version', label: 'Version' },
		{ value: 'none', label: 'None' }
	];
	let nodeButtons = $state<HTMLButtonElement[]>([]);
	function onNodeKeydown(e: KeyboardEvent) {
		const step = { ArrowRight: 1, ArrowLeft: -1, ArrowDown: 1, ArrowUp: -1 }[e.key];
		if (!step) return;
		e.preventDefault();
		const i = nodeModes.findIndex((m) => m.value === nodeMode);
		const next = (i + step + nodeModes.length) % nodeModes.length;
		setNodeMode(nodeModes[next]!.value);
		nodeButtons[next]?.focus();
	}

	const input = 'h-10 bg-background px-3 dark:bg-background';
	const mono = 'font-mono text-[13px] md:text-[13px]';
	const providers: [Provider, string][] = [
		['github', 'GitHub'],
		['gitea', 'Gitea']
	];
</script>

<svelte:window onscroll={updateActive} onresize={updateActive} />

{#snippet fieldError(field: Field)}
	{#if errors[field]}<p class="text-[13px] text-destructive">{errors[field]}</p>{/if}
{/snippet}

<PageHeader title="Project Settings" />

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
				aria-current={active === s.id ? 'location' : undefined}
			>
				{s.label}
			</a>
		{/each}
	</nav>

	{#if !draft || !project.data}
		<div class="flex flex-col gap-8">
			<Skeleton class="h-52 rounded-xl" />
			<Skeleton class="h-52 rounded-xl" />
		</div>
	{:else}
		{@const p = project.data}
		<div class="flex min-w-0 flex-col gap-8">
			<section id="general" class="flex scroll-mt-32 flex-col gap-8" aria-label="General">
				<SettingsCard
					title="Project Name"
					description="Used to identify this project across degoploy."
					note="Use at most 100 characters."
					dirty={dirty('name')}
					saving={saving === 'name'}
					error={cardError('name')}
					onsave={() => save('name')}
				>
					<Label for="set-name" class="sr-only">Project name</Label>
					<Input
						id="set-name"
						bind:value={draft.name}
						maxlength={100}
						class={cn(input, 'max-w-[400px]')}
						aria-invalid={!!errors.name}
					/>
					{@render fieldError('name')}
				</SettingsCard>

				<SettingsCard
					title="Project Slug"
					description="Names the project's data directory and is part of its webhook URL."
					note="The slug can't be changed after the project is created."
				>
					<div
						class="flex h-10 max-w-[400px] items-center gap-2 rounded-md border border-input pr-3 font-mono text-[13px] text-muted-foreground/80"
						aria-label="Project slug"
					>
						<span class="flex h-full items-center border-r border-input px-3">projects/</span>
						<LockIcon class="size-[15px] shrink-0" />
						<span class="truncate">{p.slug}</span>
					</div>
				</SettingsCard>

				<SettingsCard
					title="Accept Webhooks"
					description="When turned off, incoming pushes are still logged but never start a deployment."
					note="Manual “Deploy now” keeps working while this is off."
					dirty={dirty('webhooks')}
					saving={saving === 'webhooks'}
					error={cardError('webhooks')}
					onsave={() => save('webhooks')}
				>
					<div class="flex items-center gap-2">
						<Switch
							id="set-enabled"
							bind:checked={draft.enabled}
							class="data-[size=default]:h-5 data-[size=default]:w-[34px]"
						/>
						<Label for="set-enabled" class="text-[13px] font-medium">
							{draft.enabled ? 'Enabled' : 'Disabled'}
						</Label>
					</div>
					{@render fieldError('enabled')}
				</SettingsCard>
			</section>

			<section id="repository" class="flex scroll-mt-32 flex-col gap-8" aria-label="Repository">
				<SettingsCard
					title="Git Repository"
					description="Where degoploy clones from. Use the SSH URL for private repositories; the project's deploy key is used to authenticate."
					note="Changing the URL keeps the existing working directory."
					dirty={dirty('repository')}
					saving={saving === 'repository'}
					error={cardError('repository')}
					onsave={() => save('repository')}
				>
					<div
						class="inline-flex h-9 gap-0.5 self-start rounded-md border border-input p-[3px]"
						role="radiogroup"
						aria-label="Git provider"
					>
						{#each providers as [value, label] (value)}
							{@const on = draft.provider === value}
							<button
								type="button"
								role="radio"
								aria-checked={on}
								class={cn(
									'flex items-center gap-1.5 rounded-sm px-3.5 text-[13px] font-medium transition-colors [&_svg]:size-3.5',
									on ? 'bg-accent text-foreground' : 'text-muted-foreground hover:text-foreground'
								)}
								onclick={() => (draft!.provider = value)}
							>
								<ProviderIcon provider={value} />
								{label}
							</button>
						{/each}
					</div>
					<Label for="set-repo" class="sr-only">Repository URL</Label>
					<Input
						id="set-repo"
						bind:value={draft.repo_url}
						spellcheck={false}
						autocomplete="off"
						placeholder="git@github.com:owner/repo.git"
						class={cn(input, mono, 'max-w-[560px]')}
						aria-invalid={!!errors.repo_url}
					/>
					{@render fieldError('provider')}
					{@render fieldError('repo_url')}
				</SettingsCard>

				<SettingsCard
					title="Working Directory"
					description="Absolute path on this server. The repository is cloned here on the first deploy."
					note="The deploy script runs in this directory."
					dirty={dirty('workdir')}
					saving={saving === 'workdir'}
					error={cardError('workdir')}
					onsave={() => save('workdir')}
				>
					<Label for="set-workdir" class="sr-only">Working directory</Label>
					<div class="relative max-w-[560px]">
						<FolderIcon
							class="pointer-events-none absolute top-1/2 left-3 size-[15px] -translate-y-1/2 text-muted-foreground/80"
						/>
						<Input
							id="set-workdir"
							bind:value={draft.work_dir}
							spellcheck={false}
							autocomplete="off"
							class={cn(input, mono, 'pl-9')}
							aria-invalid={!!errors.work_dir}
						/>
					</div>
					{@render fieldError('work_dir')}
				</SettingsCard>
			</section>

			<section
				id="deploy-script"
				class="flex scroll-mt-32 flex-col gap-8"
				aria-label="Deploy Script"
			>
				<SettingsCard
					title="Deploy Script"
					description="Choose where the script comes from and what happens before it runs."
					note="Every save of a managed script is kept as a revision."
					dirty={dirty('script')}
					saving={saving === 'script'}
					error={cardError('script')}
					onsave={() => save('script')}
				>
					<div class="grid gap-3 sm:grid-cols-2" role="radiogroup" aria-label="Script source">
						{#each [{ value: 'managed', title: 'Managed by degoploy', desc: 'Edit it in the Script tab.' }, { value: 'repo', title: 'From the repository', desc: 'Read from the repo after each pull.' }] as const as o (o.value)}
							{@const on = draft.script_source === o.value}
							<button
								type="button"
								role="radio"
								aria-checked={on}
								class={cn(
									'flex gap-3 rounded-lg border p-4 text-left transition-colors',
									on
										? 'border-muted-foreground bg-muted'
										: 'border-input bg-background hover:border-muted-foreground/60'
								)}
								onclick={() => (draft!.script_source = o.value)}
							>
								<span
									class={cn(
										'mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-full border-[1.5px]',
										on ? 'border-foreground' : 'border-muted-foreground/80'
									)}
								>
									{#if on}<span class="size-2 rounded-full bg-foreground"></span>{/if}
								</span>
								<span class="flex flex-col gap-1">
									<span
										class={cn(
											'text-sm font-medium',
											on ? 'text-foreground' : 'text-muted-foreground'
										)}>{o.title}</span
									>
									<span class="text-[13px] text-muted-foreground/80">{o.desc}</span>
								</span>
							</button>
						{/each}
					</div>
					{@render fieldError('script_source')}

					{#if draft.script_source === 'repo'}
						<div class="flex flex-col gap-2">
							<Label for="set-script-path" class="text-[13px] text-muted-foreground"
								>Script path in the repository</Label
							>
							<Input
								id="set-script-path"
								bind:value={draft.script_path}
								spellcheck={false}
								placeholder=".degoploy/deploy.sh"
								class={cn(input, mono, 'max-w-[400px]')}
								aria-invalid={!!errors.script_path}
							/>
							{@render fieldError('script_path')}
						</div>
					{/if}

					<div class="flex items-center gap-3 pt-1">
						<Switch
							id="set-autopull"
							bind:checked={draft.auto_pull}
							disabled={draft.script_source === 'repo'}
							class="data-[size=default]:h-5 data-[size=default]:w-[34px]"
						/>
						<div class="flex flex-col gap-0.5">
							<Label for="set-autopull" class="text-sm">Update repository before running</Label>
							<span class="text-[13px] text-muted-foreground/80">
								{draft.script_source === 'repo'
									? 'Always on: the script is read from the pulled repository.'
									: 'Clone, or fetch and check out the pushed commit.'}
							</span>
						</div>
					</div>
					{@render fieldError('auto_pull')}
				</SettingsCard>

				<SettingsCard
					title="Node.js"
					description="The Node.js version on the deploy script's PATH, with npm, npx and corepack."
					dirty={dirty('node')}
					saving={saving === 'node'}
					error={cardError('node')}
					onsave={() => save('node')}
				>
					{#snippet note()}
						{#if p.node_used}
							The latest deployment ran with Node.js <span class="font-mono">{p.node_used}</span>.
						{:else}
							Installed on the first deployment that needs it.
						{/if}
					{/snippet}
					<div
						class="grid h-10 max-w-[400px] grid-cols-3 gap-0.5 rounded-md border border-input p-[3px]"
						role="radiogroup"
						aria-label="Node.js version"
					>
						{#each nodeModes as m, i (m.value)}
							{@const on = nodeMode === m.value}
							<button
								bind:this={nodeButtons[i]}
								type="button"
								role="radio"
								aria-checked={on}
								tabindex={on ? 0 : -1}
								class={cn(
									'rounded-sm text-[13px] font-medium transition-colors outline-none focus-visible:ring-3 focus-visible:ring-ring/50',
									on ? 'bg-accent text-foreground' : 'text-muted-foreground hover:text-foreground'
								)}
								onclick={() => setNodeMode(m.value)}
								onkeydown={onNodeKeydown}>{m.label}</button
							>
						{/each}
					</div>

					{#if nodeMode === 'auto'}
						<p class="text-[13px] text-muted-foreground/80">
							Reads <span class="font-mono">.nvmrc</span>,
							<span class="font-mono">.node-version</span> or
							<span class="font-mono">engines.node</span> in
							<span class="font-mono">package.json</span> from the repository.
							{#if nodeStatus.data?.default}
								Without one, the default version (Node.js {nodeStatus.data.default}) is used.
							{:else}
								Without one, scripts use the server's own node.
							{/if}
						</p>
					{:else if nodeMode === 'version'}
						<div class="flex flex-col gap-2">
							<Label for="set-node" class="sr-only">Node.js version</Label>
							<div class="flex flex-wrap items-center gap-2">
								<Input
									id="set-node"
									bind:value={draft.node_version}
									spellcheck={false}
									autocomplete="off"
									placeholder="22"
									class={cn(input, mono, 'w-[200px]')}
									aria-invalid={!!errors.node_version}
								/>
								{#each nodePicks as v (v)}
									<button
										type="button"
										class={cn(
											'h-7 rounded-md border px-2.5 font-mono text-xs transition-colors',
											draft.node_version === v
												? 'border-muted-foreground bg-muted text-foreground'
												: 'border-input text-muted-foreground hover:text-foreground'
										)}
										onclick={() => (draft!.node_version = v)}>{v}</button
									>
								{/each}
							</div>
							<p class="text-[13px] text-muted-foreground/80">
								A major version such as 22, an exact one such as 20.11.1, a range such as
								<span class="font-mono">&gt;=20</span>, or lts.
							</p>
						</div>
					{:else}
						<p class="text-[13px] text-muted-foreground/80">
							degoploy leaves PATH as it is: scripts use the node installed on the server, if any.
						</p>
					{/if}
					{@render fieldError('node_version')}
					{#if canNode}
						<a
							href={resolve('/(app)/node')}
							class="self-start text-[13px] text-muted-foreground underline-offset-2 hover:text-foreground hover:underline"
							>Manage installed versions</a
						>
					{/if}
				</SettingsCard>

				<SettingsCard
					title="Timeout"
					description="Runs longer than this are stopped and marked as timed out."
					note="Between 10 seconds and 24 hours."
					dirty={dirty('timeout')}
					saving={saving === 'timeout'}
					error={cardError('timeout')}
					onsave={() => save('timeout')}
				>
					<div class="flex flex-wrap items-center gap-3">
						<Label for="set-timeout" class="sr-only">Timeout in seconds</Label>
						<div class="relative w-[200px]">
							<Input
								id="set-timeout"
								type="number"
								min={10}
								max={86400}
								bind:value={draft.timeout_sec}
								class={cn(input, mono, 'pr-20')}
								aria-invalid={!!errors.timeout_sec}
							/>
							<span
								class="pointer-events-none absolute top-1/2 right-3 -translate-y-1/2 text-[13px] text-muted-foreground/80"
								>seconds</span
							>
						</div>
						{#if Number(draft.timeout_sec) >= 60}
							<span class="text-[13px] text-muted-foreground/80"
								>= {formatDuration(Number(draft.timeout_sec) * 1000)}</span
							>
						{/if}
					</div>
					{@render fieldError('timeout_sec')}
				</SettingsCard>
			</section>

			{#if canSecrets}
				<section
					id="deploy-key"
					class="flex scroll-mt-32 flex-col gap-8"
					aria-label="Deploy Key & Webhook"
				>
					{#if secrets.isError}
						<p class="text-sm text-destructive">{secrets.error.message}</p>
					{/if}
					<DeployKeyCard
						project={p}
						secrets={secrets.data}
						onregenerate={() => regenerate('key')}
					/>
					<WebhookCard
						project={p}
						secrets={secrets.data}
						onregenerate={() => regenerate('secret')}
					/>
				</section>
			{/if}

			<section id="danger-zone" class="scroll-mt-32" aria-label="Danger Zone">
				<SettingsCard
					title="Delete Project"
					note="This action is not reversible. Please be certain."
					danger
				>
					{#snippet description()}
						Permanently removes the project, its deploy key, managed script, revisions, variables
						and deployment history. Files in <span class="font-mono text-[13px] break-all"
							>{p.work_dir}</span
						>
						are left untouched.
					{/snippet}
					{#snippet action()}
						<Button
							variant="destructive"
							class="h-8 shrink-0 bg-destructive px-3.5 text-[13px] text-white hover:bg-destructive/90 dark:bg-destructive dark:hover:bg-destructive/90"
							onclick={() => {
								confirmSlug = '';
								deleteOpen = true;
							}}
						>
							Delete
						</Button>
					{/snippet}
				</SettingsCard>
			</section>
		</div>

		<AlertDialog.Root bind:open={deleteOpen}>
			<AlertDialog.Content>
				<AlertDialog.Header>
					<AlertDialog.Title>Delete "{p.name}"?</AlertDialog.Title>
					<AlertDialog.Description>
						This cannot be undone. Type <code class="font-semibold">{p.slug}</code> to confirm.
					</AlertDialog.Description>
				</AlertDialog.Header>
				<div class="flex flex-col gap-2">
					<Label for="confirm-slug" class="sr-only">Project slug</Label>
					<Input id="confirm-slug" bind:value={confirmSlug} autocomplete="off" class="font-mono" />
				</div>
				<AlertDialog.Footer>
					<AlertDialog.Cancel disabled={deleting}>Cancel</AlertDialog.Cancel>
					<Button
						variant="destructive"
						disabled={confirmSlug !== p.slug || deleting}
						onclick={remove}
					>
						{#if deleting}<LoaderIcon class="animate-spin" />{/if}
						Delete project
					</Button>
				</AlertDialog.Footer>
			</AlertDialog.Content>
		</AlertDialog.Root>
	{/if}
</div>
