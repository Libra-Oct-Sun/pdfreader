<script lang="ts">
  export let name = ''
  export let size = 18
</script>

<svg
  width={size}
  height={size}
  viewBox="0 0 24 24"
  fill="none"
  stroke="currentColor"
  stroke-width="1.8"
  stroke-linecap="round"
  stroke-linejoin="round"
  aria-hidden="true"
>
  {#if name === 'back'}
    <path d="M15 18l-6-6 6-6" />
  {:else if name === 'pen'}
    <path d="M4 20l4.2-1 10.6-10.6a2.1 2.1 0 0 0-3-3L5.2 16 4 20z" />
    <path d="M14.5 6.5l3 3" />
  {:else if name === 'highlighter'}
    <path d="M9 14l-4 4v3h4l4-4" />
    <path d="M12 11l6-6 4 4-6 6-4-4z" />
    <path d="M5 21h14" />
  {:else if name === 'eraser'}
    <path d="M4 20h16" />
    <path d="M6.5 17.5l10-10a2.1 2.1 0 0 1 3 3l-10 10" />
    <path d="M9 15l3 3" />
  {:else if name === 'undo'}
    <path d="M9 14L4 9l5-5" />
    <path d="M4 9h10a6 6 0 0 1 0 12h-3" />
  {:else if name === 'redo'}
    <path d="M15 14l5-5-5-5" />
    <path d="M20 9H10a6 6 0 0 0 0 12h3" />
  {:else if name === 'minus'}
    <path d="M5 12h14" />
  {:else if name === 'plus'}
    <path d="M12 5v14M5 12h14" />
  {:else if name === 'fit'}
    <path d="M8 3H3v5M16 3h5v5M8 21H3v-5M16 21h5v-5" />
  {:else if name === 'open'}
    <path d="M3 7h6l2 2h10v10H3V7z" />
    <path d="M3 7V5h7l2 2" />
  {:else if name === 'file'}
    <path d="M6 3h8l4 4v14H6V3z" />
    <path d="M14 3v5h5" />
    <path d="M9 13h6M9 17h6" />
  {:else if name === 'trash'}
    <path d="M4 7h16M9 7V4h6v3M7 7l1 14h8l1-14" />
  {:else if name === 'edit'}
    <path d="M4 20h4l10-10-4-4L4 16v4z" />
    <path d="M13 6l4 4" />
  {:else if name === 'left'}
    <path d="M15 18l-6-6 6-6" />
  {:else if name === 'right'}
    <path d="M9 18l6-6-6-6" />
  {:else if name === 'check'}
    <path d="M5 12l4 4L19 6" />
  {:else if name === 'close'}
    <path d="M6 6l12 12M18 6L6 18" />
  {:else if name === 'book'}
    <path d="M4 5.5A2.5 2.5 0 0 1 6.5 3H20v16H6.5A2.5 2.5 0 0 0 4 21.5v-16z" />
    <path d="M4 18.5A2.5 2.5 0 0 1 6.5 16H20" />
  {:else if name === 'bookmark'}
    <path d="M6 4h12v17l-6-4-6 4V4z" />
  {:else if name === 'bookmark-filled'}
    <path d="M6 4h12v17l-6-4-6 4V4z" fill="currentColor" stroke="none" />
  {:else if name === 'export'}
    <path d="M12 3v12" />
    <path d="M7 8l5-5 5 5" />
    <path d="M4 15v5h16v-5" />
  {:else if name === 'sidebar'}
    <rect x="3" y="4" width="18" height="16" rx="2" />
    <path d="M9 4v16" />
  {:else if name === 'text'}
    <path d="M5 5h14M12 5v14M8 19h8" />
  {/if}
</svg>
