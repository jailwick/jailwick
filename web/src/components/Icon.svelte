<script lang="ts">
  import { css } from 'styled-system/css'
  // Line icons (24×24, stroke), drawn for jailwick in the style of Lucide.
  const paths: Record<string, string> = {
    projects: 'M3 7l9-4 9 4-9 4-9-4zM3 7v10l9 4 9-4V7M12 11v10',
    sessions: 'M4 5h16v11H4zM8 20h8M12 16v4M7 9l2 2-2 2M11 13h4',
    workspaces: 'M12 3l9 5-9 5-9-5 9-5zM3 13l9 5 9-5M3 17l9 5 9-5',
    network: 'M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6l8-3zM9 12l2 2 4-4',
    images: 'M12 3a9 9 0 100 18 9 9 0 000-18zM12 9a3 3 0 100 6 3 3 0 000-6z',
    settings: 'M12 15a3 3 0 100-6 3 3 0 000 6zM19.4 15a1.7 1.7 0 00.3 1.8l.1.1a2 2 0 11-2.8 2.8l-.1-.1a1.7 1.7 0 00-1.8-.3 1.7 1.7 0 00-1 1.5V21a2 2 0 11-4 0v-.1a1.7 1.7 0 00-1.1-1.5 1.7 1.7 0 00-1.8.3l-.1.1a2 2 0 11-2.8-2.8l.1-.1a1.7 1.7 0 00.3-1.8 1.7 1.7 0 00-1.5-1H3a2 2 0 110-4h.1a1.7 1.7 0 001.5-1.1 1.7 1.7 0 00-.3-1.8l-.1-.1a2 2 0 112.8-2.8l.1.1a1.7 1.7 0 001.8.3H9a1.7 1.7 0 001-1.5V3a2 2 0 114 0v.1a1.7 1.7 0 001 1.5 1.7 1.7 0 001.8-.3l.1-.1a2 2 0 112.8 2.8l-.1.1a1.7 1.7 0 00-.3 1.8V9a1.7 1.7 0 001.5 1H21a2 2 0 110 4h-.1a1.7 1.7 0 00-1.5 1z',
    play: 'M7 5l12 7-12 7V5z',
    stop: 'M6 6h12v12H6z',
    restart: 'M3 12a9 9 0 0115.5-6.2L21 8M21 3v5h-5M21 12a9 9 0 01-15.5 6.2L3 16M3 21v-5h5',
    terminal: 'M4 17l6-5-6-5M12 19h8',
    external: 'M14 4h6v6M20 4l-9 9M18 14v5a1 1 0 01-1 1H5a1 1 0 01-1-1V7a1 1 0 011-1h5',
    plus: 'M12 5v14M5 12h14',
    chevron: 'M9 6l6 6-6 6',
    chevronDown: 'M6 9l6 6 6-6',
    bell: 'M6 8a6 6 0 0112 0c0 7 3 9 3 9H3s3-2 3-9M10.3 21a1.94 1.94 0 003.4 0',
    back: 'M15 18l-6-6 6-6',
    copy: 'M9 9h11v11H9zM5 15H4V4h11v1',
    info: 'M12 3a9 9 0 100 18 9 9 0 000-18zM12 11v5M12 8h.01',
    keyboard: 'M3 6h18v12H3zM7 10h.01M11 10h.01M15 10h.01M7 14h10',
    book: 'M4 5a2 2 0 012-2h13v16H6a2 2 0 00-2 2V5zM4 19a2 2 0 012-2h13',
    sun: 'M12 16a4 4 0 100-8 4 4 0 000 8zM12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4',
    moon: 'M21 12.8A9 9 0 1111.2 3a7 7 0 009.8 9.8z',
    monitor: 'M3 4h18v12H3zM8 20h8M12 16v4',
    check: 'M20 6L9 17l-5-5',
    x: 'M18 6L6 18M6 6l12 12',
    alert: 'M12 9v4M12 17h.01M10.3 3.9L1.8 18a2 2 0 001.7 3h17a2 2 0 001.7-3L13.7 3.9a2 2 0 00-3.4 0z',
    branch: 'M6 3v12M18 9a3 3 0 100-6 3 3 0 000 6zM6 21a3 3 0 100-6 3 3 0 000 6zM18 9a9 9 0 01-9 9',
    database: 'M12 3c4.4 0 8 1.3 8 3s-3.6 3-8 3-8-1.3-8-3 3.6-3 8-3zM4 6v12c0 1.7 3.6 3 8 3s8-1.3 8-3V6M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3',
    folder: 'M3 6a2 2 0 012-2h4l2 2h8a2 2 0 012 2v10a2 2 0 01-2 2H5a2 2 0 01-2-2V6z',
    lock: 'M5 11h14v10H5zM8 11V7a4 4 0 118 0v4',
    inbox: 'M22 12h-6l-2 3h-4l-2-3H2M5.5 5h13L22 12v7a2 2 0 01-2 2H4a2 2 0 01-2-2v-7l3.5-7z',
    eyeoff: 'M3 3l18 18M10.6 10.6a2 2 0 002.8 2.8M9.9 5.1A10 10 0 0112 5c6 0 10 7 10 7a17 17 0 01-3.2 4M6.6 6.6C3.9 8.4 2 12 2 12s4 7 10 7a9.8 9.8 0 005.4-1.6',
    flame: 'M12 22c4 0 7-3 7-7 0-5-5-8-6-13-2 3-2 5-1 7-2-1-3-3-3-5-3 3-4 6-4 9 0 5 3 9 7 9z',
    refresh: 'M21 12a9 9 0 11-3-6.7L21 8M21 3v5h-5',
    spinner: 'M21 12a9 9 0 11-9-9',
    pause: 'M9 5v14M15 5v14',
    search: 'M11 19a8 8 0 100-16 8 8 0 000 16zM21 21l-4.3-4.3',
  }
  let { name, size = 16, class: cls = '' }: { name: string; size?: number; class?: string } = $props()
  const spin = css({ animation: 'spin 0.9s linear infinite' })
</script>

<svg class="{cls} {name === 'spinner' ? spin : ''}" width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"
  stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
  <path d={paths[name] ?? ''} />
</svg>
