<script lang="ts">
	import { useQueryClient } from '@tanstack/svelte-query';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import { projectKeys, projectsApi } from '#lib/api/projects.js';
	import PageHeader from '#lib/components/page-header.svelte';
	import ProjectForm, {
		emptyProject,
		type ProjectFormValues
	} from '#lib/components/project-form.svelte';
	import { Button } from '#lib/components/ui/button/index.js';

	const queryClient = useQueryClient();

	async function create(values: ProjectFormValues) {
		const p = await projectsApi.create(values);
		queryClient.setQueryData(projectKeys.detail(p.id), p);
		await queryClient.invalidateQueries({ queryKey: projectKeys.all, exact: true });
		toast.success(`Project "${p.name}" created`);
		await goto(resolve('/(app)/projects/[id]', { id: String(p.id) }));
	}
</script>

<PageHeader
	title="New project"
	description="degoploy generates an SSH deploy key and webhook secret for it."
/>

<ProjectForm initial={emptyProject()} mode="create" submitLabel="Create project" onsubmit={create}>
	{#snippet actions()}
		<Button variant="ghost" href={resolve('/(app)/projects')}>Cancel</Button>
	{/snippet}
</ProjectForm>
