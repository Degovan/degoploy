<script lang="ts" module>
	import type { CreateProjectRequest, Project } from '#lib/api/types.gen.js';
	import { session } from '#lib/stores/session.svelte.js';

	export type ProjectFormValues = CreateProjectRequest;

	export const emptyProject = (): ProjectFormValues => ({
		name: '',
		slug: '',
		provider: 'github',
		repo_url: '',
		work_dir: '',
		script_source: 'managed',
		script_path: '.degoploy/deploy.sh',
		auto_pull: true,
		timeout_sec: 600,
		enabled: true,
		releases: false,
		default_branch: 'main'
	});

	/** The folder degoploy picks for a project without one; empty when it picks none. */
	export const defaultWorkDir = (slug: string): string => {
		const dir = session.me?.projects_dir;
		return dir ? `${dir}/${slug}` : '';
	};

	/** Form values for an existing project (only the editable fields). */
	export const projectToForm = (p: Project): ProjectFormValues => ({
		name: p.name,
		slug: p.slug,
		provider: p.provider,
		repo_url: p.repo_url,
		// The default folder shows as the placeholder, so the field stays empty.
		work_dir: p.work_dir === defaultWorkDir(p.slug) ? '' : p.work_dir,
		script_source: p.script_source,
		script_path: p.script_path || '.degoploy/deploy.sh',
		auto_pull: p.auto_pull,
		timeout_sec: p.timeout_sec,
		enabled: p.enabled,
		releases: p.releases,
		default_branch: ''
	});
</script>

<script lang="ts">
	import type { Snippet } from 'svelte';
	import { ApiError, errorMessage } from '#lib/api/client.js';
	import type { Provider } from '#lib/api/types.gen.js';
	import ProviderIcon from '#lib/components/provider-icon.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { Switch } from '#lib/components/ui/switch/index.js';
	import * as RadioGroup from '#lib/components/ui/radio-group/index.js';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import { slugify } from '#lib/repo.js';
	import { cn } from '#lib/utils.js';
	import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import FolderIcon from '@lucide/svelte/icons/folder';
	import GitBranchIcon from '@lucide/svelte/icons/git-branch';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';

	let {
		initial,
		mode,
		title,
		description,
		submitLabel,
		onsubmit,
		actions,
		aside
	}: {
		initial: ProjectFormValues;
		mode: 'create' | 'edit';
		title: string;
		description?: string;
		submitLabel: string;
		onsubmit: (values: ProjectFormValues) => Promise<void>;
		actions?: Snippet;
		/** Rendered beside the form card with the live values (e.g. a summary of what happens next). */
		aside?: Snippet<[ProjectFormValues]>;
	} = $props();

	// The form owns a copy; `initial` only seeds it.
	// svelte-ignore state_referenced_locally
	let v = $state<ProjectFormValues>({ ...initial });
	// svelte-ignore state_referenced_locally
	let slugTouched = $state(mode === 'edit');
	let errors = $state<Record<string, string>>({});
	let formError = $state('');
	let submitting = $state(false);

	let scriptOpen = $state(true);
	// svelte-ignore state_referenced_locally
	let advancedOpen = $state(mode === 'edit');

	const scriptFields = ['script_source', 'script_path', 'auto_pull', 'releases', 'timeout_sec'];
	const projectsDir = $derived(session.me?.projects_dir ?? '');

	$effect(() => {
		if (!slugTouched) v.slug = slugify(v.name);
	});
	$effect(() => {
		// Repository scripts only exist after a pull, and each release is one.
		if (v.script_source === 'repo' || v.releases) v.auto_pull = true;
	});

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		submitting = true;
		errors = {};
		formError = '';
		try {
			await onsubmit({ ...v, timeout_sec: Number(v.timeout_sec) });
		} catch (err) {
			if (err instanceof ApiError && err.field) {
				errors = { [err.field]: err.message };
				// Show the field the error is about.
				if (scriptFields.includes(err.field)) scriptOpen = true;
				if (err.field === 'enabled') advancedOpen = true;
			} else formError = errorMessage(err);
		} finally {
			submitting = false;
		}
	}

	const providers: [Provider, string][] = [
		['github', 'GitHub'],
		['gitea', 'Gitea']
	];
	const hint = 'text-xs text-muted-foreground';
</script>

{#snippet fieldError(name: string)}
	{#if errors[name]}
		<p class="text-sm text-destructive">{errors[name]}</p>
	{/if}
{/snippet}

{#snippet sectionToggle(label: string, open: boolean, toggle: () => void, controls: string)}
	<button
		type="button"
		class="flex w-full items-center gap-3 px-5 py-4 text-left font-medium"
		aria-expanded={open}
		aria-controls={controls}
		onclick={toggle}
	>
		<ChevronDownIcon
			class={cn('size-4 text-muted-foreground transition-transform', !open && '-rotate-90')}
		/>
		{label}
	</button>
{/snippet}

<div class={cn('grid items-start gap-8', aside && 'lg:grid-cols-[minmax(0,1fr)_22rem]')}>
	<!-- A field's server error clears as soon as the user edits that field. -->
	<form
		class="overflow-hidden rounded-xl border bg-card"
		onsubmit={submit}
		oninput={(e) => {
			const id = (e.target as HTMLElement).id;
			if (errors[id]) errors = Object.fromEntries(Object.entries(errors).filter(([k]) => k !== id));
		}}
	>
		<div class="flex flex-col gap-1 border-b px-6 py-5">
			<h2 class="text-lg font-semibold tracking-tight">{title}</h2>
			{#if description}<p class="text-sm text-muted-foreground">{description}</p>{/if}
		</div>

		<div class="flex flex-col gap-6 p-6">
			{#if formError}
				<Alert.Root variant="destructive">
					<CircleAlertIcon />
					<Alert.Title>{formError}</Alert.Title>
				</Alert.Root>
			{/if}

			<div class="grid gap-6 sm:grid-cols-2">
				<div class="flex flex-col gap-2">
					<Label for="name">Project Name</Label>
					<Input
						id="name"
						required
						bind:value={v.name}
						aria-invalid={!!errors.name}
						placeholder="my-app"
						class="h-10"
					/>
					{@render fieldError('name')}
				</div>
				<div class="flex flex-col gap-2">
					<Label for="slug">Slug</Label>
					<div
						class={cn(
							'flex h-10 overflow-hidden rounded-md border border-input focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/50 dark:bg-input/30',
							errors.slug && 'border-destructive',
							mode === 'edit' && 'opacity-60'
						)}
					>
						<span
							class="flex items-center border-r bg-muted/50 px-3 font-mono text-sm text-muted-foreground"
							>projects/</span
						>
						<input
							id="slug"
							required
							bind:value={v.slug}
							oninput={() => (slugTouched = true)}
							disabled={mode === 'edit'}
							aria-invalid={!!errors.slug}
							placeholder="my-app"
							class="min-w-0 flex-1 bg-transparent px-3 font-mono text-sm outline-none placeholder:text-muted-foreground disabled:cursor-not-allowed"
						/>
					</div>
					{#if mode === 'edit'}
						<p class={hint}>The slug names the project's data directory and cannot change.</p>
					{/if}
					{@render fieldError('slug')}
				</div>
			</div>

			<div class="flex flex-col gap-2">
				<span class="text-sm font-medium" id="provider-label">Git Provider</span>
				<div
					class="grid grid-cols-2 gap-1 rounded-lg border p-1"
					role="radiogroup"
					aria-labelledby="provider-label"
				>
					{#each providers as [value, label] (value)}
						<button
							type="button"
							role="radio"
							id={`provider-${value}`}
							aria-checked={v.provider === value}
							class={cn(
								'flex h-9 items-center justify-center gap-2 rounded-md text-sm transition-colors',
								v.provider === value
									? 'bg-accent font-medium text-foreground shadow-xs'
									: 'text-muted-foreground hover:text-foreground'
							)}
							onclick={() => (v.provider = value)}
						>
							<ProviderIcon provider={value} />
							{label}
						</button>
					{/each}
				</div>
				{@render fieldError('provider')}
			</div>

			<div class="flex flex-col gap-2">
				<Label for="repo_url">Repository URL</Label>
				<div class="relative">
					<GitBranchIcon
						class="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground"
					/>
					<Input
						id="repo_url"
						required
						bind:value={v.repo_url}
						aria-invalid={!!errors.repo_url}
						class="h-10 pl-9 font-mono"
						placeholder={v.provider === 'gitea'
							? 'git@gitea.example.com:owner/repo.git'
							: 'git@github.com:owner/repo.git'}
					/>
				</div>
				<p class={hint}>Use the SSH URL for private repositories.</p>
				{@render fieldError('repo_url')}
			</div>

			<div class="flex flex-col gap-2">
				<Label for="work_dir">Working Directory</Label>
				<div class="relative">
					<FolderIcon
						class="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground"
					/>
					<Input
						id="work_dir"
						required={!projectsDir}
						bind:value={v.work_dir}
						aria-invalid={!!errors.work_dir}
						class="h-10 pl-9 font-mono"
						placeholder={defaultWorkDir(v.slug || 'my-app') || '/var/www/my-app'}
					/>
				</div>
				<p class={hint}>
					{#if projectsDir}
						Leave empty to use <code>{projectsDir}/{v.slug || '<slug>'}</code>, which degoploy
						prepares itself. Another folder needs write access for the user deployments run as.
					{:else}
						Absolute path on this server. The repository is cloned here on the first deploy.
					{/if}
				</p>
				{@render fieldError('work_dir')}
			</div>

			{#if mode === 'create'}
				<div class="flex flex-col gap-2 sm:max-w-[calc(50%-0.75rem)]">
					<Label for="default_branch">Branch to Deploy</Label>
					<Input
						id="default_branch"
						bind:value={v.default_branch}
						aria-invalid={!!errors.default_branch}
						class="h-10 font-mono"
						placeholder="main"
					/>
					<p class={hint}>Creates the first trigger rule. Leave empty to add rules later.</p>
					{@render fieldError('default_branch')}
				</div>
			{/if}

			<div class="overflow-hidden rounded-lg border">
				{@render sectionToggle(
					'Deploy Script',
					scriptOpen,
					() => (scriptOpen = !scriptOpen),
					'script-section'
				)}
				{#if scriptOpen}
					<div id="script-section" class="flex flex-col gap-5 border-t bg-muted/20 p-5">
						<RadioGroup.Root bind:value={v.script_source} class="grid gap-3 sm:grid-cols-2">
							{#each [['managed', 'Managed by degoploy', 'Edit in the Script tab, with revisions.'], ['repo', 'From the repository', 'Read from the repo after each pull.']] as [value, label, desc] (value)}
								<Label
									for={`source-${value}`}
									class="flex cursor-pointer items-start gap-3 rounded-lg border bg-card p-4 font-normal transition-colors has-data-checked:border-foreground/40 has-data-checked:bg-accent/40"
								>
									<RadioGroup.Item {value} id={`source-${value}`} class="mt-0.5" />
									<span class="flex flex-col gap-1">
										<span class="font-medium">{label}</span>
										<span class="text-xs text-muted-foreground">{desc}</span>
									</span>
								</Label>
							{/each}
						</RadioGroup.Root>
						{@render fieldError('script_source')}

						{#if v.script_source === 'repo'}
							<div class="flex flex-col gap-2">
								<Label for="script_path">Script path in repository</Label>
								<Input
									id="script_path"
									required
									bind:value={v.script_path}
									aria-invalid={!!errors.script_path}
									class="h-10 font-mono"
									placeholder=".degoploy/deploy.sh"
								/>
								{@render fieldError('script_path')}
							</div>
						{/if}

						<div class="flex items-center justify-between gap-4">
							<div class="flex flex-col gap-0.5">
								<Label for="auto_pull">Update repository before running</Label>
								<p class={hint}>
									Clone, or <code>git fetch</code> + checkout of the pushed commit.
									{#if v.script_source === 'repo'}Required for repository scripts.{/if}
								</p>
							</div>
							<Switch
								id="auto_pull"
								bind:checked={v.auto_pull}
								disabled={v.script_source === 'repo' || v.releases}
							/>
						</div>
						{@render fieldError('auto_pull')}

						<div class="flex items-center justify-between gap-4">
							<div class="flex flex-col gap-0.5">
								<Label for="releases">Deploy in releases</Label>
								<p class={hint}>
									Each deployment builds in <code>releases/&lt;id&gt;</code> and
									<code>current</code> switches to it when the script succeeds. Point the web server
									at <code>current</code>.
								</p>
							</div>
							<Switch id="releases" bind:checked={v.releases} />
						</div>
						{@render fieldError('releases')}

						<div class="flex flex-col gap-2">
							<Label for="timeout_sec">Timeout</Label>
							<div
								class={cn(
									'flex h-10 w-48 overflow-hidden rounded-md border border-input focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/50 dark:bg-input/30',
									errors.timeout_sec && 'border-destructive'
								)}
							>
								<span
									class="flex items-center border-r bg-muted/50 px-3 font-mono text-sm text-muted-foreground"
									>sec</span
								>
								<input
									id="timeout_sec"
									type="number"
									min={10}
									max={86400}
									required
									bind:value={v.timeout_sec}
									aria-invalid={!!errors.timeout_sec}
									class="min-w-0 flex-1 bg-transparent px-3 font-mono text-sm outline-none"
								/>
							</div>
							{@render fieldError('timeout_sec')}
						</div>
					</div>
				{/if}
			</div>

			<div class="overflow-hidden rounded-lg border">
				{@render sectionToggle(
					'Advanced',
					advancedOpen,
					() => (advancedOpen = !advancedOpen),
					'advanced-section'
				)}
				{#if advancedOpen}
					<div id="advanced-section" class="flex flex-col gap-4 border-t bg-muted/20 p-5">
						<div class="flex items-center justify-between gap-4">
							<div class="flex flex-col gap-0.5">
								<Label for="enabled">Enabled</Label>
								<p class={hint}>Disabled projects ignore incoming webhooks.</p>
							</div>
							<Switch id="enabled" bind:checked={v.enabled} />
						</div>
						{@render fieldError('enabled')}
					</div>
				{/if}
			</div>
		</div>

		<div
			class={cn(
				'flex items-center gap-2 border-t px-6 py-5',
				mode === 'edit' && 'flex-row-reverse justify-start'
			)}
		>
			<Button type="submit" disabled={submitting} class={cn('h-10', mode === 'create' && 'flex-1')}>
				{#if submitting}<LoaderIcon class="animate-spin" />{/if}
				{submitLabel}
			</Button>
			{@render actions?.()}
		</div>
	</form>

	{#if aside}
		<aside class="flex flex-col gap-4 lg:sticky lg:top-24">{@render aside(v)}</aside>
	{/if}
</div>
