<script lang="ts">
	import { useQueryClient } from '@tanstack/svelte-query';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import { projectKeys, projectsApi } from '#lib/api/projects.js';
	import ProjectForm, {
		emptyProject,
		type ProjectFormValues
	} from '#lib/components/project-form.svelte';
	import { providerLabel } from '#lib/repo.js';
	import InfoIcon from '@lucide/svelte/icons/info';
	import PlusIcon from '@lucide/svelte/icons/plus';

	const queryClient = useQueryClient();

	async function create(values: ProjectFormValues) {
		const p = await projectsApi.create(values);
		queryClient.setQueryData(projectKeys.detail(p.id), p);
		await queryClient.invalidateQueries({ queryKey: projectKeys.all, exact: true });
		toast.success(`Project "${p.name}" created`);
		await goto(resolve('/(app)/projects/[id]', { id: String(p.id) }));
	}

	function steps(v: ProjectFormValues): [string, string][] {
		const host = providerLabel[v.provider];
		const branch = v.default_branch.trim();
		return [
			['Deploy key generated', `Add it on ${host} as a read-only deploy key.`],
			['Webhook URL and secret', 'Paste both into the repository’s webhook settings.'],
			branch
				? ['First trigger rule', `Pushes to ${branch} start a deployment.`]
				: ['Trigger rules', 'Add rules in the Triggers tab to choose what deploys.'],
			v.script_source === 'managed'
				? ['Starter deploy script', 'A commented template, editable in the Script tab.']
				: ['Deploy script from the repo', `${v.script_path || 'The script'} runs after each pull.`]
		];
	}
</script>

<svelte:head>
	<title>New Project · degoploy</title>
</svelte:head>

<section class="hero-grid relative overflow-hidden border-b">
	{#each ['top-8 left-[8%]', 'top-24 left-[62%]', 'bottom-8 right-[8%]'] as pos (pos)}
		<PlusIcon
			class="absolute hidden size-6 stroke-1 text-muted-foreground/50 md:block {pos}"
			aria-hidden="true"
		/>
	{/each}
	<div class="relative container-page flex flex-col gap-4 py-16 md:py-20">
		<h1 class="text-4xl font-semibold tracking-tighter md:text-6xl">Let’s deploy something new.</h1>
		<p class="max-w-2xl text-muted-foreground md:text-lg">
			Point degoploy at a repository. It pulls the code and runs your script on every matching push.
		</p>
	</div>
</section>

<div class="container-page py-10">
	<ProjectForm
		initial={emptyProject()}
		mode="create"
		title="Configure Project"
		description="degoploy generates an SSH deploy key and a webhook secret when you create it."
		submitLabel="Create Project"
		onsubmit={create}
	>
		{#snippet aside(v)}
			<div class="rounded-xl border bg-card p-6">
				<h2 class="mb-5 font-semibold">What happens next</h2>
				<ol class="flex flex-col">
					{#each steps(v) as [title, text], i (title)}
						<li class="relative flex gap-4 pb-6 last:pb-0">
							{#if i < 3}
								<span class="absolute top-8 bottom-1 left-3 w-px bg-border" aria-hidden="true"
								></span>
							{/if}
							<span
								class="flex size-6 shrink-0 items-center justify-center rounded-full border font-mono text-xs text-muted-foreground"
								>{i + 1}</span
							>
							<div class="flex flex-col gap-1 pt-0.5">
								<span class="text-sm font-medium">{title}</span>
								<span class="text-sm text-muted-foreground">{text}</span>
							</div>
						</li>
					{/each}
				</ol>
			</div>
			<p class="flex gap-2.5 px-1 text-sm text-muted-foreground">
				<InfoIcon class="mt-0.5 size-4 shrink-0" />
				You can run “Deploy now” before the webhook is connected to check the script.
			</p>
		{/snippet}
	</ProjectForm>
</div>
