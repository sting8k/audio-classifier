<script lang="ts">
  const PATHS = {
    logo: 'M3 12h2M7 8v8M11 5v14M15 9v6M19 11v2',
    folder: 'M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z',
    plus: 'M12 5v14M5 12h14',
    x: 'M6 6l12 12M18 6L6 18',
    play: 'M7 5v14l12-7z',
    pause: 'M7 5h4v14H7zM13 5h4v14h-4z',
    download: 'M12 4v11M7 10l5 5 5-5M5 20h14',
    search: 'M5 11a6 6 0 1 0 12 0a6 6 0 1 0-12 0M20 20l-4.5-4.5',
    prev: 'M15 6l-6 6 6 6',
    next: 'M9 6l6 6-6 6',
  }
  let { name, size = 16 }: { name: keyof typeof PATHS; size?: number } = $props()
  const filled = $derived(name === 'play' || name === 'pause')
</script>

<svg width={size} height={size} viewBox="0 0 24 24" fill={filled ? 'currentColor' : 'none'} stroke={filled ? 'none' : 'currentColor'} stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
  <path d={PATHS[name]} />
</svg>
