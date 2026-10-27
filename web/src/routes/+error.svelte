<script lang="ts">
	import { page } from '$app/state';
	import { Button } from '#lib/components/ui/button/index.js';
</script>

<svelte:head>
	<title>{page.status} · degoploy</title>
</svelte:head>

<div class="flex min-h-svh flex-col items-center justify-center gap-4 p-6 text-center">
	<p class="text-5xl font-semibold tracking-tight">{page.status}</p>
	<p class="text-muted-foreground">{page.error?.message ?? 'Something went wrong'}</p>
	<Button href="/projects" variant="outline">Back to projects</Button>
</div>
