<script lang="ts" module>
	import type { User } from '#lib/api/types.gen.js';

	// Deterministic gradient per user, so people are recognizable at a glance.
	const gradients = [
		['#52a8ff', '#50e3c2'],
		['#ff6166', '#ffb224'],
		['#a87fff', '#52a8ff'],
		['#50e3c2', '#0072f5'],
		['#f75f8f', '#a87fff'],
		['#ffb224', '#50e3c2']
	] as const;

	export function initials(u: Pick<User, 'display_name' | 'username'>): string {
		return (u.display_name || u.username)
			.split(/[\s._-]+/)
			.filter(Boolean)
			.slice(0, 2)
			.map((p) => p[0]!.toUpperCase())
			.join('');
	}

	function gradientFor(seed: string): string {
		// FNV-1a: a plain ×31 hash degrades to the character sum modulo 6.
		let h = 0x811c9dc5;
		for (let i = 0; i < seed.length; i++) h = Math.imul(h ^ seed.charCodeAt(i), 0x01000193);
		const [a, b] = gradients[(h >>> 0) % gradients.length]!;
		return `linear-gradient(135deg, ${a}, ${b})`;
	}
</script>

<script lang="ts">
	import { cn } from '#lib/utils.js';

	let { user, class: className }: { user: User; class?: string } = $props();
</script>

<span
	aria-hidden="true"
	class={cn(
		'flex size-8 shrink-0 items-center justify-center rounded-full text-xs font-semibold text-black/80',
		className
	)}
	style:background={gradientFor(user.username)}
>
	{initials(user)}
</span>
