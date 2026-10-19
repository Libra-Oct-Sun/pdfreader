<script lang="ts">
  import { tick } from 'svelte'
  import type { TextNote, Tool } from './types'

  export let pageIndex: number
  export let cssWidth: number
  export let cssHeight: number
  export let notes: TextNote[] = []
  export let mode: 'read' | 'annotate' = 'read'
  export let tool: Tool = 'pen'
  export let selectedID: string | null = null
  export let editingID: string | null = null
  export let onSelect: (id: string | null) => void = () => {}
  export let onEdit: (id: string) => void = () => {}
  export let onUpdate: (id: string, patch: Partial<TextNote>) => void = () => {}
  export let onDelete: (id: string) => void = () => {}
  export let onCreate: (page: number, x: number, y: number) => void = () => {}
  export let onFinishEdit: () => void = () => {}

  type DragState = {
    id: string
    pointerId: number
    startX: number
    startY: number
    noteX: number
    noteY: number
    moved: boolean
  }

  let drag: DragState | null = null
  $: active = mode === 'annotate' && tool === 'text'

  function createAt(event: PointerEvent) {
    if (!active) return
    const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
    const x = clamp01((event.clientX - rect.left) / Math.max(1, rect.width))
    const y = clamp01((event.clientY - rect.top) / Math.max(1, rect.height))
    onCreate(pageIndex, x, y)
  }

  function handlePointerDown(event: PointerEvent, note: TextNote) {
    if (!active || editingID === note.id) return
    event.stopPropagation()
    event.preventDefault()
    onSelect(note.id)
    drag = {
      id: note.id,
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      noteX: note.x,
      noteY: note.y,
      moved: false
    }
    ;(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId)
  }

  function handlePointerMove(event: PointerEvent, note: TextNote) {
    if (!drag || drag.id !== note.id || drag.pointerId !== event.pointerId) return
    const dx = (event.clientX - drag.startX) / Math.max(1, cssWidth)
    const dy = (event.clientY - drag.startY) / Math.max(1, cssHeight)
    if (Math.abs(dx) > 0.002 || Math.abs(dy) > 0.002) drag.moved = true
    if (!drag.moved) return
    event.preventDefault()
    onUpdate(note.id, {
      x: clamp(drag.noteX + dx, 0, Math.max(0, 1 - note.width)),
      y: clamp(drag.noteY + dy, 0, 0.92)
    })
  }

  function handlePointerUp(event: PointerEvent, note: TextNote) {
    if (!drag || drag.id !== note.id) return
    const element = event.currentTarget as HTMLElement
    if (element.hasPointerCapture(event.pointerId)) {
      element.releasePointerCapture(event.pointerId)
    }
    drag = null
  }

  function focusTextArea(node: HTMLTextAreaElement) {
    void tick().then(() => {
      node.focus()
      node.selectionStart = node.value.length
      node.selectionEnd = node.value.length
      resizeTextArea(node)
    })
  }

  function resizeTextArea(node: HTMLTextAreaElement) {
    node.style.height = 'auto'
    node.style.height = `${node.scrollHeight}px`
  }

  function handleTextInput(event: Event, note: TextNote) {
    const target = event.currentTarget as HTMLTextAreaElement
    onUpdate(note.id, { text: target.value })
    resizeTextArea(target)
  }

  function handleTextKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape') {
      event.preventDefault()
      onFinishEdit()
    }
  }

  function clamp01(value: number) {
    return clamp(value, 0, 1)
  }

  function clamp(value: number, min: number, max: number) {
    return Math.min(max, Math.max(min, value))
  }
</script>

<div class="text-layer" class:active>
  {#if active}
    <button
      type="button"
      class="text-click-catcher"
      aria-label="在页面上添加文字"
      on:pointerdown={createAt}
    ></button>
  {/if}

  {#each notes as note (note.id)}
    <div
      class="text-note"
      class:selected={selectedID === note.id}
      class:editing={editingID === note.id}
      role="button"
      tabindex="0"
      aria-label={note.text || '空白文字批注'}
      style:left={`${note.x * cssWidth}px`}
      style:top={`${note.y * cssHeight}px`}
      style:width={`${note.width * cssWidth}px`}
      style:font-size={`${Math.max(7, note.size * cssWidth)}px`}
      style:color={note.color}
      on:pointerdown={(event) => handlePointerDown(event, note)}
      on:pointermove={(event) => handlePointerMove(event, note)}
      on:pointerup={(event) => handlePointerUp(event, note)}
      on:pointercancel={(event) => handlePointerUp(event, note)}
      on:dblclick|stopPropagation={() => onEdit(note.id)}
      on:keydown={(event) => {
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault()
          onEdit(note.id)
        }
      }}
    >
      {#if editingID === note.id}
        <textarea
          value={note.text}
          placeholder="输入文字…"
          rows="1"
          use:focusTextArea
          on:input={(event) => handleTextInput(event, note)}
          on:keydown={handleTextKeydown}
          on:blur={onFinishEdit}
          on:pointerdown|stopPropagation
        ></textarea>
      {:else}
        <div class="text-content">{note.text || '双击编辑文字'}</div>
      {/if}

      {#if selectedID === note.id || editingID === note.id}
        <button
          class="text-delete"
          title="删除文字"
          aria-label="删除文字"
          on:pointerdown|stopPropagation|preventDefault={() => {}}
          on:click|stopPropagation={() => onDelete(note.id)}
        >
          ×
        </button>
      {/if}
    </div>
  {/each}
</div>

<style>
  .text-layer {
    position: absolute;
    inset: 0;
    z-index: 4;
    pointer-events: none;
  }

  .text-layer.active {
    pointer-events: auto;
  }

  .text-click-catcher {
    position: absolute;
    inset: 0;
    padding: 0;
    border: 0;
    background: transparent;
    cursor: text;
  }

  .text-note {
    position: absolute;
    box-sizing: border-box;
    padding: 0.32em 0.46em;
    border: 1px solid transparent;
    border-radius: 5px;
    background: rgba(255, 252, 245, 0.94);
    box-shadow: 0 2px 8px rgba(32, 36, 43, 0.08);
    line-height: 1.38;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    cursor: move;
    user-select: none;
    pointer-events: none;
  }

  .text-layer.active .text-note {
    pointer-events: auto;
  }

  .text-note.selected,
  .text-note.editing {
    border-color: #c8553d;
    box-shadow:
      0 0 0 2px rgba(200, 85, 61, 0.15),
      0 4px 14px rgba(32, 36, 43, 0.12);
  }

  .text-note.editing {
    cursor: text;
    user-select: text;
  }

  .text-content {
    min-height: 1.38em;
  }

  .text-note textarea {
    display: block;
    width: 100%;
    min-height: 1.38em;
    margin: 0;
    padding: 0;
    border: 0;
    outline: 0;
    overflow: hidden;
    resize: none;
    background: transparent;
    color: inherit;
    font: inherit;
    line-height: inherit;
    user-select: text;
  }

  .text-note textarea::placeholder {
    color: rgba(32, 36, 43, 0.38);
  }

  .text-delete {
    position: absolute;
    right: -9px;
    top: -9px;
    width: 18px;
    height: 18px;
    display: grid;
    place-items: center;
    padding: 0;
    border: 1px solid #fff;
    border-radius: 50%;
    background: #c8553d;
    color: #fff;
    font-size: 11px;
    line-height: 1;
    cursor: pointer;
    box-shadow: 0 2px 6px rgba(32, 36, 43, 0.2);
  }
</style>
