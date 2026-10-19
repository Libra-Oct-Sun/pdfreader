<script lang="ts">
  import { onDestroy, onMount, tick } from 'svelte'
  import * as pdfjsLib from 'pdfjs-dist'
  import type { PDFDocumentLoadingTask, PDFDocumentProxy } from 'pdfjs-dist'
  import pdfWorkerUrl from 'pdfjs-dist/build/pdf.worker.min.mjs?url'
  import PdfPage from './PdfPage.svelte'
  import Icon from './Icon.svelte'
  import type { DocInfo, ReadingState, Stroke, Tool } from './types'
  import {
    GetPDFURL,
    GetReadingState,
    LoadStrokes,
    SaveReadingState,
    SaveStrokes,
    SetPageCount
  } from '../../wailsjs/go/main/App.js'

  pdfjsLib.GlobalWorkerOptions.workerSrc = pdfWorkerUrl

  export let doc: DocInfo
  export let onClose: () => void = () => {}
  export let onDocUpdate: (doc: DocInfo) => void = () => {}

  type PageSize = { index: number; width: number; height: number }
  type HistoryOp =
    | { type: 'add'; stroke: Stroke }
    | { type: 'remove'; stroke: Stroke; index: number }

  const colors = ['#1f2937', '#c8553d', '#2f6bff', '#238b63', '#d99a00']

  let scroller: HTMLElement
  let pagesHost: HTMLDivElement

  let loading = true
  let loadingText = '正在打开文档…'
  let error = ''
  let pdf: PDFDocumentProxy | null = null
  let loadingTask: PDFDocumentLoadingTask | null = null
  let pdfUrl = ''
  let pageSizes: PageSize[] = []
  let maxPageWidth = 612
  let fitScale = 1
  let zoom = 1

  let mode: 'read' | 'annotate' = 'read'
  let tool: Tool = 'pen'
  let color = colors[0]
  let penWidth = 2
  let strokes: Stroke[] = []
  let undoStack: HistoryOp[] = []
  let redoStack: HistoryOp[] = []

  let currentPage = 1
  let furthestPage = 1
  let pageInput = 1
  let resumeToast = ''
  let passwordNeeded = false
  let passwordInput = ''
  let passwordError = ''
  let suppressSave = false

  let scrollTimer: ReturnType<typeof setTimeout> | undefined
  let strokeTimer: ReturnType<typeof setTimeout> | undefined
  let resizeTimer: ReturnType<typeof setTimeout> | undefined
  let resumeTimer: ReturnType<typeof setTimeout> | undefined
  let saveQueue: Promise<void> = Promise.resolve()
  let destroyed = false
  let strokesLoaded = false

  let initialState: ReadingState = {
    docId: doc.id,
    page: 1,
    offset: 0,
    zoom: 1,
    furthestPage: 1,
    updatedAt: 0
  }
  let savedStrokes: Stroke[] = []

  $: totalPages = pageSizes.length || doc.pageCount || 1
  $: contentWidth = maxPageWidth * fitScale * zoom
  $: progress =
    totalPages > 0
      ? Math.min(100, (Math.max(furthestPage, currentPage) / totalPages) * 100)
      : 0

  onMount(() => {
    void init()
    scroller?.addEventListener('wheel', handleWheel, { passive: false })
    window.addEventListener('keydown', handleKeydown)
    window.addEventListener('resize', handleResize)
    document.addEventListener('visibilitychange', handleVisibility)
    return () => {
      destroyed = true
      window.removeEventListener('keydown', handleKeydown)
      window.removeEventListener('resize', handleResize)
      document.removeEventListener('visibilitychange', handleVisibility)
      scroller?.removeEventListener('wheel', handleWheel)
      if (scrollTimer) clearTimeout(scrollTimer)
      if (strokeTimer) clearTimeout(strokeTimer)
      if (resizeTimer) clearTimeout(resizeTimer)
      if (resumeTimer) clearTimeout(resumeTimer)
      void loadingTask?.destroy()
    }
  })

  onDestroy(() => {
    void loadingTask?.destroy()
  })

  async function init() {
    try {
      strokesLoaded = false
      loading = true
      loadingText = '正在打开文档…'
      const [urlResult, stateResult, strokesResult] = await Promise.all([
        GetPDFURL(doc.id),
        GetReadingState(doc.id),
        LoadStrokes(doc.id)
      ])
      pdfUrl = String(urlResult || '')
      if (!pdfUrl) throw new Error('无法读取 PDF 地址')
      initialState = stateResult as unknown as ReadingState
      savedStrokes = (strokesResult as unknown as Stroke[]) || []

      await openPdf(pdfUrl)
      finishOpen()
    } catch (reason: any) {
      if (reason?.name === 'PasswordException') {
        passwordNeeded = true
        loading = false
        return
      }
      error = readableError(reason)
      loading = false
    }
  }

  async function openPdf(url: string, password?: string) {
    loading = true
    error = ''
    if (loadingTask) {
      await loadingTask.destroy()
      loadingTask = null
    }

    const task = pdfjsLib.getDocument({
      url,
      password,
      cMapUrl: '/pdfjs/cmaps/',
      cMapPacked: true,
      standardFontDataUrl: '/pdfjs/standard_fonts/',
      wasmUrl: '/pdfjs/wasm/',
      iccUrl: '/pdfjs/iccs/'
    })
    loadingTask = task
    const loaded = await task.promise
    if (destroyed) {
      await task.destroy()
      return
    }
    pdf = loaded
    await SetPageCount(doc.id, loaded.numPages)
    pageSizes = await collectPageSizes(loaded)
    maxPageWidth = Math.max(...pageSizes.map((page) => page.width), 1)
    fitScale = computeFitScale()
    loading = false
  }

  function finishOpen() {
    strokes = savedStrokes
    strokesLoaded = true
    undoStack = []
    redoStack = []
    currentPage = clamp(Math.round(initialState.page || 1), 1, totalPages)
    furthestPage = clamp(
      Math.max(initialState.furthestPage || 1, currentPage),
      1,
      totalPages
    )
    pageInput = currentPage
    zoom = clamp(initialState.zoom || 1, 0.5, 4)
    void tick().then(() => {
      requestAnimationFrame(() => {
        scrollToPosition({
          page: currentPage,
          offset: clamp01(initialState.offset || 0)
        })
        if (
          initialState.updatedAt > 0 &&
          (currentPage > 1 || (initialState.offset || 0) > 0.02)
        ) {
          showResume(`已回到第 ${currentPage} 页`)
        }
      })
    })
  }

  async function submitPassword() {
    passwordError = ''
    try {
      await openPdf(pdfUrl, passwordInput)
      passwordNeeded = false
      finishOpen()
    } catch (reason: any) {
      if (reason?.name === 'PasswordException') {
        passwordError = '密码不正确，请再试一次'
      } else {
        passwordError = readableError(reason)
      }
    }
  }

  async function collectPageSizes(loaded: PDFDocumentProxy): Promise<PageSize[]> {
    const total = loaded.numPages
    const sizes: PageSize[] = new Array(total)
    const batchSize = 24
    for (let start = 0; start < total; start += batchSize) {
      if (destroyed) break
      const end = Math.min(total, start + batchSize)
      loadingText = `正在读取页面 ${end} / ${total}`
      const jobs: Promise<void>[] = []
      for (let index = start; index < end; index += 1) {
        jobs.push(
          (async () => {
            try {
              const page = await loaded.getPage(index + 1)
              const viewport = page.getViewport({ scale: 1 })
              sizes[index] = {
                index: index + 1,
                width: viewport.width,
                height: viewport.height
              }
              page.cleanup()
            } catch (reason) {
              console.error('read page size', index + 1, reason)
              sizes[index] = { index: index + 1, width: 612, height: 792 }
            }
          })()
        )
      }
      await Promise.all(jobs)
    }
    return sizes
  }

  function computeFitScale() {
    const width = scroller?.clientWidth || window.innerWidth || 1000
    return Math.max(0.15, Math.max(320, width - 64) / Math.max(1, maxPageWidth))
  }

  function handleResize() {
    if (resizeTimer) clearTimeout(resizeTimer)
    resizeTimer = setTimeout(() => {
      if (!scroller || loading) return
      const position = computePosition()
      fitScale = computeFitScale()
      void tick().then(() => {
        requestAnimationFrame(() => {
          scrollToPosition(position)
          window.setTimeout(savePosition, 420)
        })
      })
    }, 120)
  }

  function handleWheel(event: WheelEvent) {
    if (!event.ctrlKey) return
    event.preventDefault()
    void applyZoom(zoom + (event.deltaY < 0 ? 0.1 : -0.1))
  }

  async function applyZoom(next: number) {
    if (!pdf || loading) return
    const position = computePosition()
    zoom = clamp(Number(next.toFixed(2)), 0.5, 4)
    await tick()
    requestAnimationFrame(() => {
      scrollToPosition(position)
      window.setTimeout(savePosition, 440)
    })
  }

  function scrollToPosition(position: { page: number; offset: number }) {
    if (!scroller || !pagesHost) return
    const page = clamp(Math.round(position.page), 1, totalPages)
    const element = pagesHost.querySelector<HTMLElement>(`[data-page="${page}"]`)
    if (!element) return
    const top =
      element.getBoundingClientRect().top -
      scroller.getBoundingClientRect().top +
      scroller.scrollTop
    suppressSave = true
    scroller.scrollTop = Math.max(
      0,
      top + clamp01(position.offset) * element.offsetHeight
    )
    window.setTimeout(() => {
      suppressSave = false
    }, 360)
  }

  function computePosition(): { page: number; offset: number } {
    if (!scroller || !pagesHost) return { page: 1, offset: 0 }
    const probe = scroller.getBoundingClientRect().top + 12
    const elements = pagesHost.querySelectorAll<HTMLElement>('[data-page]')
    let page = 1
    let offset = 0
    for (const element of elements) {
      const rect = element.getBoundingClientRect()
      const index = Number(element.dataset.page)
      if (rect.top <= probe && rect.bottom > probe) {
        page = index
        offset = clamp01((probe - rect.top) / Math.max(1, rect.height))
        break
      }
      if (rect.top > probe) {
        page = index
        offset = 0
        break
      }
      page = index
      offset = 1
    }
    return { page, offset }
  }

  function handleScroll() {
    if (suppressSave || loading) return
    const position = computePosition()
    currentPage = position.page
    pageInput = position.page
    if (position.page > furthestPage) furthestPage = position.page
    if (scrollTimer) clearTimeout(scrollTimer)
    scrollTimer = setTimeout(savePosition, 460)
  }

  function savePosition() {
    if (!pdf || suppressSave || destroyed) return
    const position = computePosition()
    const state: ReadingState = {
      docId: doc.id,
      page: position.page,
      offset: position.offset,
      zoom,
      furthestPage: Math.max(furthestPage, position.page),
      updatedAt: Date.now()
    }
    void SaveReadingState(state as any).catch((reason) => {
      console.error('save reading state', reason)
    })
  }

  function jumpToPage(value: number) {
    if (!pdf) return
    const page = clamp(Math.round(value || 1), 1, totalPages)
    currentPage = page
    pageInput = page
    if (page > furthestPage) furthestPage = page
    scrollToPosition({ page, offset: 0 })
    window.setTimeout(savePosition, 420)
  }

  function handlePageInput(event: KeyboardEvent) {
    if (event.key !== 'Enter') return
    event.preventDefault()
    jumpToPage(Number(pageInput))
    ;(event.target as HTMLInputElement).blur()
  }

  function addStroke(stroke: Stroke) {
    strokes = [...strokes, stroke]
    undoStack = [...undoStack.slice(-99), { type: 'add', stroke }]
    redoStack = []
    scheduleSaveStrokes()
  }

  function eraseStroke(strokeId: string) {
    const index = strokes.findIndex((stroke) => stroke.id === strokeId)
    if (index < 0) return
    const stroke = strokes[index]
    strokes = strokes.filter((item) => item.id !== strokeId)
    undoStack = [...undoStack.slice(-99), { type: 'remove', stroke, index }]
    redoStack = []
    scheduleSaveStrokes()
  }

  function undo() {
    const operation = undoStack[undoStack.length - 1]
    if (!operation) return
    undoStack = undoStack.slice(0, -1)
    if (operation.type === 'add') {
      strokes = strokes.filter((stroke) => stroke.id !== operation.stroke.id)
    } else {
      const next = [...strokes]
      next.splice(Math.min(operation.index, next.length), 0, operation.stroke)
      strokes = next
    }
    redoStack = [...redoStack, operation]
    scheduleSaveStrokes()
  }

  function redo() {
    const operation = redoStack[redoStack.length - 1]
    if (!operation) return
    redoStack = redoStack.slice(0, -1)
    if (operation.type === 'add') {
      strokes = [...strokes, operation.stroke]
    } else {
      strokes = strokes.filter((stroke) => stroke.id !== operation.stroke.id)
    }
    undoStack = [...undoStack, operation]
    scheduleSaveStrokes()
  }

  function scheduleSaveStrokes() {
    if (!strokesLoaded) return
    if (strokeTimer) clearTimeout(strokeTimer)
    strokeTimer = setTimeout(() => {
      const snapshot = strokes
      saveQueue = saveQueue
        .then(() => SaveStrokes(doc.id, snapshot as any))
        .then(() => undefined)
        .catch((reason) => {
          console.error('save strokes', reason)
        })
    }, 500)
  }

  async function flushStrokes() {
    if (!strokesLoaded) return
    if (strokeTimer) clearTimeout(strokeTimer)
    const snapshot = strokes
    saveQueue = saveQueue
      .then(() => SaveStrokes(doc.id, snapshot as any))
      .then(() => undefined)
      .catch((reason) => {
        console.error('save strokes', reason)
      })
    await saveQueue
  }

  function handleVisibility() {
    if (document.visibilityState === 'hidden') {
      savePosition()
      void flushStrokes()
    }
  }

  async function handleClose() {
    if (pdf) {
      savePosition()
      await flushStrokes()
      onDocUpdate({
        ...doc,
        lastPage: currentPage,
        offset: computePosition().offset,
        zoom,
        furthestPage
      })
    }
    onClose()
  }

  function handleKeydown(event: KeyboardEvent) {
    const target = event.target as HTMLElement | null
    if (
      target &&
      (target.tagName === 'INPUT' ||
        target.tagName === 'TEXTAREA' ||
        target.isContentEditable)
    ) {
      if (event.key === 'Escape') {
        ;(target as HTMLInputElement).blur()
      }
      return
    }

    const key = event.key.toLowerCase()
    if ((event.ctrlKey || event.metaKey) && key === 'z') {
      event.preventDefault()
      if (event.shiftKey) redo()
      else undo()
      return
    }
    if ((event.ctrlKey || event.metaKey) && key === 'y') {
      event.preventDefault()
      redo()
      return
    }
    if (event.ctrlKey || event.metaKey) return

    if (key === 'p') {
      event.preventDefault()
      mode = mode === 'annotate' ? 'read' : 'annotate'
    } else if (event.key === 'Escape') {
      mode = 'read'
    } else if (event.key === '+' || event.key === '=') {
      event.preventDefault()
      void applyZoom(zoom + 0.15)
    } else if (event.key === '-') {
      event.preventDefault()
      void applyZoom(zoom - 0.15)
    }
  }

  function showResume(message: string) {
    resumeToast = message
    if (resumeTimer) clearTimeout(resumeTimer)
    resumeTimer = setTimeout(() => {
      resumeToast = ''
    }, 3400)
  }

  function readableError(reason: any) {
    if (reason instanceof Error) return reason.message
    if (typeof reason === 'string') return reason
    if (reason?.message) return String(reason.message)
    return '打开文档时发生错误'
  }

  function clamp01(value: number) {
    if (value < 0) return 0
    if (value > 1) return 1
    return value
  }

  function clamp(value: number, min: number, max: number) {
    return Math.min(max, Math.max(min, value))
  }
</script>

<div class="reader">
  <header class="reader-bar">
    <button class="icon-button" title="返回书架" aria-label="返回书架" on:click={handleClose}>
      <Icon name="back" size={19} />
    </button>
    <div class="doc-title">
      <div class="doc-name" title={doc.path}>{doc.name}</div>
      <div class="doc-meta">第 {currentPage} / {totalPages} 页</div>
    </div>
    <div class="bar-spacer"></div>

    <div class="page-jump">
      <input
        type="number"
        min="1"
        max={totalPages}
        bind:value={pageInput}
        on:keydown={handlePageInput}
        aria-label="跳转到页码"
      />
      <span>/ {totalPages}</span>
    </div>

    <div class="zoom-group" aria-label="缩放">
      <button class="icon-button small" title="缩小" on:click={() => applyZoom(zoom - 0.15)}>
        <Icon name="minus" size={16} />
      </button>
      <button class="zoom-value" title="恢复适宽" on:click={() => applyZoom(1)}>
        {Math.round(zoom * 100)}%
      </button>
      <button class="icon-button small" title="放大" on:click={() => applyZoom(zoom + 0.15)}>
        <Icon name="plus" size={16} />
      </button>
    </div>

    <button
      class="pen-toggle"
      class:active={mode === 'annotate'}
      title={mode === 'annotate' ? '退出批注（P / Esc）' : '开始批注（P）'}
      on:click={() => (mode = mode === 'annotate' ? 'read' : 'annotate')}
    >
      <Icon name="pen" size={17} />
      <span>{mode === 'annotate' ? '批注中' : '批注'}</span>
    </button>
  </header>

  <main class="viewport" bind:this={scroller} on:scroll={handleScroll}>
    {#if pageSizes.length > 0 && pdf}
      <div class="pages-layer">
        <div class="pages" bind:this={pagesHost} style:width={`${contentWidth}px`}>
          {#each pageSizes as page (page.index)}
            {@const pageCssWidth = page.width * fitScale * zoom}
            {@const pageCssHeight = page.height * fitScale * zoom}
            <div
              class="page-slot"
              data-page={page.index}
              style:width={`${pageCssWidth}px`}
              style:height={`${pageCssHeight}px`}
            >
              <PdfPage
                {pdf}
                pageIndex={page.index}
                pageWidth={page.width}
                cssWidth={pageCssWidth}
                cssHeight={pageCssHeight}
                {mode}
                {tool}
                {color}
                {penWidth}
                {strokes}
                onStroke={addStroke}
                onErase={eraseStroke}
              />
            </div>
          {/each}
        </div>
      </div>
    {/if}
  </main>

  <div class="progress-track">
    <div class="progress-fill" style:width={`${progress}%`}></div>
  </div>

  {#if mode === 'annotate'}
    <div class="ink-toolbar">
      <button
        class="tool-button"
        class:selected={tool === 'pen'}
        title="钢笔"
        on:click={() => (tool = 'pen')}
      >
        <Icon name="pen" size={18} />
      </button>
      <button
        class="tool-button"
        class:selected={tool === 'highlighter'}
        title="荧光笔"
        on:click={() => (tool = 'highlighter')}
      >
        <Icon name="highlighter" size={18} />
      </button>
      <button
        class="tool-button"
        class:selected={tool === 'eraser'}
        title="整笔擦除"
        on:click={() => (tool = 'eraser')}
      >
        <Icon name="eraser" size={18} />
      </button>

      <div class="toolbar-divider"></div>

      {#each colors as swatch}
        <button
          class="swatch"
          class:selected={color === swatch}
          style:background={swatch}
          title={`颜色 ${swatch}`}
          aria-label={`选择颜色 ${swatch}`}
          on:click={() => {
            color = swatch
            if (tool === 'eraser') tool = 'pen'
          }}
        ></button>
      {/each}

      <div class="toolbar-divider"></div>

      <label class="width-control" title="线宽">
        <span class="width-dot" style:width={`${penWidth + 2}px`} style:height={`${penWidth + 2}px`}></span>
        <input type="range" min="1" max="8" step="0.5" bind:value={penWidth} />
      </label>

      <div class="toolbar-divider"></div>

      <button class="tool-button" title="撤销 Ctrl+Z" disabled={!undoStack.length} on:click={undo}>
        <Icon name="undo" size={18} />
      </button>
      <button class="tool-button" title="重做 Ctrl+Y" disabled={!redoStack.length} on:click={redo}>
        <Icon name="redo" size={18} />
      </button>
      <button class="done-button" on:click={() => (mode = 'read')}>
        <Icon name="check" size={16} />
        完成
      </button>
    </div>
  {/if}

  {#if resumeToast}
    <div class="resume-toast">{resumeToast}</div>
  {/if}

  {#if loading}
    <div class="overlay">
      <div class="loading-card">
        <div class="spinner"></div>
        <div>{loadingText}</div>
      </div>
    </div>
  {/if}

  {#if passwordNeeded}
    <div class="overlay">
      <form class="dialog" on:submit|preventDefault={submitPassword}>
        <div class="dialog-icon"><Icon name="book" size={24} /></div>
        <h2>这份 PDF 需要密码</h2>
        <p>输入密码后继续阅读，密码只在本机使用。</p>
        <input
          type="password"
          bind:value={passwordInput}
          placeholder="PDF 密码"
        />
        {#if passwordError}<div class="form-error">{passwordError}</div>{/if}
        <div class="dialog-actions">
          <button type="button" class="ghost-button" on:click={handleClose}>返回</button>
          <button type="submit" class="primary-button">继续</button>
        </div>
      </form>
    </div>
  {/if}

  {#if error}
    <div class="overlay">
      <div class="dialog">
        <div class="dialog-icon error"><Icon name="close" size={24} /></div>
        <h2>打不开这份文档</h2>
        <p class="error-text">{error}</p>
        <div class="dialog-actions">
          <button class="ghost-button" on:click={handleClose}>返回书架</button>
          <button class="primary-button" on:click={() => { error = ''; void init() }}>
            重试
          </button>
        </div>
      </div>
    </div>
  {/if}
</div>

<style>
  .reader {
    height: 100vh;
    display: flex;
    flex-direction: column;
    background-color: #e9e5dc;
    background-image: radial-gradient(rgba(31, 35, 40, 0.035) 0.7px, transparent 0.7px);
    background-size: 18px 18px;
    color: #20242b;
    overflow: hidden;
  }

  .reader-bar {
    height: 56px;
    flex: 0 0 56px;
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 0 12px;
    background: rgba(250, 249, 246, 0.94);
    border-bottom: 1px solid rgba(32, 36, 43, 0.1);
    backdrop-filter: blur(14px);
    z-index: 20;
  }

  .icon-button {
    width: 34px;
    height: 34px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border: 1px solid transparent;
    border-radius: 9px;
    background: transparent;
    color: #3c4149;
    cursor: pointer;
  }

  .icon-button:hover {
    background: rgba(32, 36, 43, 0.06);
  }

  .icon-button.small {
    width: 28px;
    height: 28px;
  }

  .doc-title {
    min-width: 0;
    display: flex;
    flex-direction: column;
    line-height: 1.25;
  }

  .doc-name {
    max-width: 420px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 13px;
    font-weight: 650;
    letter-spacing: 0.01em;
  }

  .doc-meta {
    font-size: 11px;
    color: #7c7a73;
    margin-top: 2px;
  }

  .bar-spacer {
    flex: 1;
  }

  .page-jump {
    display: flex;
    align-items: center;
    gap: 6px;
    height: 30px;
    padding: 0 8px;
    border-radius: 8px;
    background: rgba(32, 36, 43, 0.045);
    color: #86827a;
    font-size: 12px;
  }

  .page-jump input {
    width: 34px;
    border: 0;
    outline: 0;
    background: transparent;
    color: #20242b;
    font: inherit;
    font-weight: 650;
    text-align: right;
    appearance: textfield;
    -moz-appearance: textfield;
  }

  .page-jump input::-webkit-outer-spin-button,
  .page-jump input::-webkit-inner-spin-button {
    -webkit-appearance: none;
    margin: 0;
  }

  .zoom-group {
    display: flex;
    align-items: center;
    height: 32px;
    border-radius: 9px;
    background: rgba(32, 36, 43, 0.045);
  }

  .zoom-value {
    width: 48px;
    border: 0;
    background: transparent;
    color: #55524c;
    font-size: 11px;
    font-weight: 650;
    cursor: pointer;
  }

  .pen-toggle {
    height: 34px;
    display: inline-flex;
    align-items: center;
    gap: 7px;
    padding: 0 13px;
    border: 1px solid rgba(32, 36, 43, 0.14);
    border-radius: 9px;
    background: #fff;
    color: #34383f;
    font-size: 12px;
    font-weight: 650;
    cursor: pointer;
    transition: background 140ms ease, color 140ms ease, border-color 140ms ease;
  }

  .pen-toggle:hover {
    border-color: rgba(32, 36, 43, 0.3);
  }

  .pen-toggle.active {
    background: #20242b;
    border-color: #20242b;
    color: #fff;
  }

  .viewport {
    flex: 1;
    min-height: 0;
    overflow: auto;
    scrollbar-gutter: stable;
    padding: 26px 32px 90px;
    outline: none;
  }

  .pages-layer {
    min-height: 100%;
    display: flex;
    justify-content: center;
  }

  .pages {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 22px;
  }

  .page-slot {
    flex: 0 0 auto;
  }

  .progress-track {
    position: absolute;
    left: 0;
    right: 0;
    bottom: 0;
    height: 3px;
    background: rgba(32, 36, 43, 0.08);
    z-index: 18;
  }

  .progress-fill {
    height: 100%;
    background: #c8553d;
    transition: width 180ms ease;
  }

  .ink-toolbar {
    position: absolute;
    left: 50%;
    bottom: 22px;
    transform: translateX(-50%);
    display: flex;
    align-items: center;
    gap: 5px;
    padding: 7px 9px;
    border-radius: 14px;
    background: #20242b;
    color: #f6f4ef;
    box-shadow: 0 12px 40px rgba(20, 22, 26, 0.3);
    z-index: 30;
  }

  .tool-button {
    width: 34px;
    height: 34px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border: 0;
    border-radius: 9px;
    background: transparent;
    color: #d8d5cd;
    cursor: pointer;
  }

  .tool-button:hover {
    background: rgba(255, 255, 255, 0.1);
  }

  .tool-button.selected {
    background: #c8553d;
    color: #fff;
  }

  .tool-button:disabled {
    opacity: 0.3;
    cursor: default;
  }

  .toolbar-divider {
    width: 1px;
    height: 22px;
    margin: 0 3px;
    background: rgba(255, 255, 255, 0.14);
  }

  .swatch {
    width: 18px;
    height: 18px;
    padding: 0;
    border: 2px solid transparent;
    border-radius: 50%;
    cursor: pointer;
    box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.3);
  }

  .swatch.selected {
    border-color: #fff;
    transform: scale(1.08);
  }

  .width-control {
    display: flex;
    align-items: center;
    gap: 7px;
    padding: 0 4px;
  }

  .width-dot {
    display: block;
    border-radius: 50%;
    background: #f6f4ef;
  }

  .width-control input {
    width: 62px;
    accent-color: #c8553d;
  }

  .done-button {
    height: 32px;
    display: inline-flex;
    align-items: center;
    gap: 5px;
    margin-left: 3px;
    padding: 0 11px;
    border: 0;
    border-radius: 8px;
    background: #f6f4ef;
    color: #20242b;
    font-size: 11px;
    font-weight: 700;
    cursor: pointer;
  }

  .resume-toast {
    position: absolute;
    top: 70px;
    left: 50%;
    transform: translateX(-50%);
    padding: 8px 15px;
    border-radius: 999px;
    background: rgba(32, 36, 43, 0.9);
    color: #fff;
    font-size: 12px;
    box-shadow: 0 8px 26px rgba(20, 22, 26, 0.18);
    z-index: 25;
    animation: toast-in 180ms ease-out;
  }

  @keyframes toast-in {
    from {
      opacity: 0;
      transform: translate(-50%, -5px);
    }
    to {
      opacity: 1;
      transform: translate(-50%, 0);
    }
  }

  .overlay {
    position: fixed;
    inset: 0;
    display: grid;
    place-items: center;
    background: rgba(246, 244, 239, 0.82);
    backdrop-filter: blur(8px);
    z-index: 60;
  }

  .loading-card {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 16px 20px;
    border-radius: 14px;
    background: #fff;
    border: 1px solid rgba(32, 36, 43, 0.08);
    box-shadow: 0 18px 50px rgba(20, 22, 26, 0.12);
    color: #55524c;
    font-size: 13px;
  }

  .spinner {
    width: 18px;
    height: 18px;
    border: 2px solid rgba(32, 36, 43, 0.14);
    border-top-color: #c8553d;
    border-radius: 50%;
    animation: spin 700ms linear infinite;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  .dialog {
    width: min(380px, calc(100vw - 48px));
    padding: 24px;
    border-radius: 18px;
    background: #fff;
    border: 1px solid rgba(32, 36, 43, 0.08);
    box-shadow: 0 24px 70px rgba(20, 22, 26, 0.16);
    text-align: left;
  }

  .dialog-icon {
    width: 42px;
    height: 42px;
    display: grid;
    place-items: center;
    border-radius: 12px;
    background: #f3eee5;
    color: #c8553d;
    margin-bottom: 14px;
  }

  .dialog-icon.error {
    background: #fbe9e5;
    color: #b23a2a;
  }

  .dialog h2 {
    margin: 0 0 7px;
    font-size: 17px;
    font-weight: 700;
    letter-spacing: -0.01em;
  }

  .dialog p {
    margin: 0 0 16px;
    color: #77736b;
    font-size: 12px;
    line-height: 1.6;
  }

  .dialog input {
    width: 100%;
    box-sizing: border-box;
    height: 38px;
    padding: 0 12px;
    border: 1px solid rgba(32, 36, 43, 0.16);
    border-radius: 9px;
    outline: none;
    font: inherit;
    font-size: 13px;
  }

  .dialog input:focus {
    border-color: #c8553d;
    box-shadow: 0 0 0 3px rgba(200, 85, 61, 0.12);
  }

  .form-error {
    margin-top: 8px;
    color: #b23a2a;
    font-size: 11px;
  }

  .dialog-actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    margin-top: 18px;
  }

  .ghost-button,
  .primary-button {
    height: 34px;
    padding: 0 14px;
    border-radius: 9px;
    font-size: 12px;
    font-weight: 650;
    cursor: pointer;
  }

  .ghost-button {
    border: 1px solid rgba(32, 36, 43, 0.14);
    background: #fff;
    color: #4f4b45;
  }

  .primary-button {
    border: 1px solid #20242b;
    background: #20242b;
    color: #fff;
  }

  .error-text {
    word-break: break-word;
    max-height: 120px;
    overflow: auto;
  }
</style>
