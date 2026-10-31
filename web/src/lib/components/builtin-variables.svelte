<script lang="ts">
	import type { Snippet } from 'svelte';
	import { toast } from 'svelte-sonner';
	import { copyText } from '#lib/clipboard.js';

	// What the runner exports to every deploy script (see scriptEnv in internal/deploy).
	const variables: [string, string][] = [
		['DEPLOY_ID', 'Deployment number'],
		['DEPLOY_PROJECT', 'Project slug'],
		['DEPLOY_PROJECT_NAME', 'Project name'],
		['DEPLOY_RULE', 'Name of the matched rule'],
		['DEPLOY_TRIGGER', 'webhook, manual, redeploy or rollback'],
		['DEPLOY_REF', 'Full git ref that was pushed'],
		['DEPLOY_BRANCH', 'Branch name, empty for tags'],
		['DEPLOY_TAG', 'Tag name, empty for branches'],
		['DEPLOY_COMMIT', 'Full commit SHA'],
		['DEPLOY_AUTHOR', 'Commit author'],
		['DEPLOY_WORKDIR', 'Folder the script runs in'],
		['DEPLOY_RELEASE', 'With releases: the release number'],
		['DEPLOY_RELEASE_DIR', 'With releases: its folder'],
		['DEPLOY_SHARED_DIR', 'With releases: the shared folder'],
		['DEPLOY_CURRENT_DIR', 'With releases: the link to the live release'],
		['GIT_SSH_COMMAND', "Uses this project's deploy key"]
	];

	let {
		title = 'Environment',
		description = 'Available to every run. Click a name to copy.',
		children
	}: { title?: string; description?: string; children?: Snippet } = $props();

	async function copy(name: string) {
		if (await copyText(name)) toast.success(`Copied ${name}`);
		else toast.error('Could not copy to the clipboard');
	}
</script>

<section class="flex flex-col gap-4 rounded-xl border bg-card p-6" aria-label={title}>
	<div class="flex flex-col gap-1">
		<h2 class="text-sm font-semibold">{title}</h2>
		<p class="text-xs text-muted-foreground/80">{description}</p>
	</div>
	<dl class="flex flex-col gap-3">
		{#each variables as [name, desc] (name)}
			<div class="flex flex-col gap-0.5">
				<dt>
					<button
						type="button"
						class="font-mono text-xs break-all text-sky-600 hover:underline dark:text-sky-400"
						onclick={() => copy(name)}
						title={`Copy ${name}`}
					>
						{name}
					</button>
				</dt>
				<dd class="text-xs text-muted-foreground/80">{desc}</dd>
			</div>
		{/each}
	</dl>
	{@render children?.()}
</section>
