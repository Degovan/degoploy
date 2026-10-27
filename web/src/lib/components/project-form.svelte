<script lang="ts" module>
	import type { CreateProjectRequest, Project } from '#lib/api/types.gen.js';

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
		default_branch: 'main'
	});

	/** Form values for an existing project (only the editable fields). */
	export const projectToForm = (p: Project): ProjectFormValues => ({
		name: p.name,
		slug: p.slug,
		provider: p.provider,
		repo_url: p.repo_url,
		work_dir: p.work_dir,
		script_source: p.script_source,
		script_path: p.script_path || '.degoploy/deploy.sh',
		auto_pull: p.auto_pull,
		timeout_sec: p.timeout_sec,
		enabled: p.enabled,
		default_branch: ''
	});
</script>

<script lang="ts">
	import type { Snippet } from 'svelte';
	import { ApiError, errorMessage } from '#lib/api/client.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { Switch } from '#lib/components/ui/switch/index.js';
	import * as RadioGroup from '#lib/components/ui/radio-group/index.js';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import { slugify } from '#lib/repo.js';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';

	let {
		initial,
		mode,
		submitLabel,
		onsubmit,
		actions
	}: {
		initial: ProjectFormValues;
		mode: 'create' | 'edit';
		submitLabel: string;
		onsubmit: (values: ProjectFormValues) => Promise<void>;
		actions?: Snippet;
	} = $props();

	// The form owns a copy; `initial` only seeds it.
	// svelte-ignore state_referenced_locally
	let v = $state<ProjectFormValues>({ ...initial });
	// svelte-ignore state_referenced_locally
	let slugTouched = $state(mode === 'edit');
	let errors = $state<Record<string, string>>({});
	let formError = $state('');
	let submitting = $state(false);

	$effect(() => {
		if (!slugTouched) v.slug = slugify(v.name);
	});
	$effect(() => {
		// Repository scripts only exist after a pull.
		if (v.script_source === 'repo') v.auto_pull = true;
	});

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		submitting = true;
		errors = {};
		formError = '';
		try {
			await onsubmit({ ...v, timeout_sec: Number(v.timeout_sec) });
		} catch (err) {
			if (err instanceof ApiError && err.field) errors = { [err.field]: err.message };
			else formError = errorMessage(err);
		} finally {
			submitting = false;
		}
	}
</script>

{#snippet fieldError(name: string)}
	{#if errors[name]}
		<p class="text-sm text-destructive">{errors[name]}</p>
	{/if}
{/snippet}

<!-- A field's server error clears as soon as the user edits that field. -->
<form
	class="flex max-w-2xl flex-col gap-8"
	onsubmit={submit}
	oninput={(e) => {
		const id = (e.target as HTMLElement).id;
		if (errors[id]) errors = Object.fromEntries(Object.entries(errors).filter(([k]) => k !== id));
	}}
>
	{#if formError}
		<Alert.Root variant="destructive">
			<CircleAlertIcon />
			<Alert.Title>{formError}</Alert.Title>
		</Alert.Root>
	{/if}

	<section class="flex flex-col gap-4">
		<h2 class="text-sm font-medium text-muted-foreground">General</h2>
		<div class="grid gap-4 sm:grid-cols-2">
			<div class="flex flex-col gap-2">
				<Label for="name">Name</Label>
				<Input
					id="name"
					required
					bind:value={v.name}
					aria-invalid={!!errors.name}
					placeholder="My App"
				/>
				{@render fieldError('name')}
			</div>
			<div class="flex flex-col gap-2">
				<Label for="slug">Slug</Label>
				<Input
					id="slug"
					required
					bind:value={v.slug}
					oninput={() => (slugTouched = true)}
					disabled={mode === 'edit'}
					aria-invalid={!!errors.slug}
					class="font-mono"
				/>
				{#if mode === 'edit'}
					<p class="text-xs text-muted-foreground">
						The slug names the project's data directory and cannot change.
					</p>
				{/if}
				{@render fieldError('slug')}
			</div>
		</div>
		<div class="flex items-center justify-between gap-4 rounded-md border p-3">
			<div class="flex flex-col gap-0.5">
				<Label for="enabled">Enabled</Label>
				<p class="text-xs text-muted-foreground">Disabled projects ignore incoming webhooks.</p>
			</div>
			<Switch id="enabled" bind:checked={v.enabled} />
		</div>
	</section>

	<section class="flex flex-col gap-4">
		<h2 class="text-sm font-medium text-muted-foreground">Repository</h2>
		<div class="flex flex-col gap-2">
			<Label>Provider</Label>
			<RadioGroup.Root bind:value={v.provider} class="flex gap-6">
				{#each [['github', 'GitHub'], ['gitea', 'Gitea']] as [value, label] (value)}
					<div class="flex items-center gap-2">
						<RadioGroup.Item {value} id={`provider-${value}`} />
						<Label for={`provider-${value}`} class="font-normal">{label}</Label>
					</div>
				{/each}
			</RadioGroup.Root>
			{@render fieldError('provider')}
		</div>
		<div class="flex flex-col gap-2">
			<Label for="repo_url">Repository URL</Label>
			<Input
				id="repo_url"
				required
				bind:value={v.repo_url}
				aria-invalid={!!errors.repo_url}
				class="font-mono"
				placeholder="git@github.com:owner/repo.git"
			/>
			<p class="text-xs text-muted-foreground">
				Use the SSH URL for private repositories; degoploy generates a deploy key for this project.
			</p>
			{@render fieldError('repo_url')}
		</div>
		<div class="flex flex-col gap-2">
			<Label for="work_dir">Working directory</Label>
			<Input
				id="work_dir"
				required
				bind:value={v.work_dir}
				aria-invalid={!!errors.work_dir}
				class="font-mono"
				placeholder="/var/www/my-app"
			/>
			<p class="text-xs text-muted-foreground">
				Absolute path on this server. The repository is cloned here on the first deploy.
			</p>
			{@render fieldError('work_dir')}
		</div>
		{#if mode === 'create'}
			<div class="flex flex-col gap-2">
				<Label for="default_branch">Branch to deploy</Label>
				<Input
					id="default_branch"
					bind:value={v.default_branch}
					aria-invalid={!!errors.default_branch}
					class="max-w-xs font-mono"
					placeholder="main"
				/>
				<p class="text-xs text-muted-foreground">
					Creates a first trigger rule for pushes to this branch. Leave empty to add rules later.
				</p>
				{@render fieldError('default_branch')}
			</div>
		{/if}
	</section>

	<section class="flex flex-col gap-4">
		<h2 class="text-sm font-medium text-muted-foreground">Deploy script</h2>
		<RadioGroup.Root bind:value={v.script_source} class="grid gap-3 sm:grid-cols-2">
			{#each [['managed', 'Managed by degoploy', 'Edit the script in this UI. Every save is kept as a revision.'], ['repo', 'From the repository', 'Run a script committed in the repo, read after each pull.']] as [value, title, desc] (value)}
				<Label
					for={`source-${value}`}
					class="flex cursor-pointer items-start gap-3 rounded-md border p-3 font-normal has-data-checked:border-primary"
				>
					<RadioGroup.Item {value} id={`source-${value}`} class="mt-0.5" />
					<span class="flex flex-col gap-1">
						<span class="font-medium">{title}</span>
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
					class="font-mono"
					placeholder=".degoploy/deploy.sh"
				/>
				{@render fieldError('script_path')}
			</div>
		{/if}

		<div class="flex items-center justify-between gap-4 rounded-md border p-3">
			<div class="flex flex-col gap-0.5">
				<Label for="auto_pull">Update repository before running</Label>
				<p class="text-xs text-muted-foreground">
					Clone or <code>git fetch</code> + <code>reset --hard</code> to the pushed commit.
					{#if v.script_source === 'repo'}Required for repository scripts.{/if}
				</p>
			</div>
			<Switch id="auto_pull" bind:checked={v.auto_pull} disabled={v.script_source === 'repo'} />
		</div>
		{@render fieldError('auto_pull')}

		<div class="flex max-w-48 flex-col gap-2">
			<Label for="timeout_sec">Timeout (seconds)</Label>
			<Input
				id="timeout_sec"
				type="number"
				min={10}
				max={86400}
				required
				bind:value={v.timeout_sec}
				aria-invalid={!!errors.timeout_sec}
			/>
			{@render fieldError('timeout_sec')}
		</div>
	</section>

	<div class="flex items-center gap-2">
		<Button type="submit" disabled={submitting}>
			{#if submitting}<LoaderIcon class="animate-spin" />{/if}
			{submitLabel}
		</Button>
		{@render actions?.()}
	</div>
</form>
