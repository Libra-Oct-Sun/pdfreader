<script lang="ts">
  import { onDestroy, onMount, tick } from 'svelte'
  import * as pdfjsLib from 'pdfjs-dist'
  import type { PDFDocumentLoadingTask, PDFDocumentProxy } from 'pdfjs-dist'
  import pdfWorkerUrl from 'pdfjs-dist/build/pdf.worker.min.mjs?url'
  import {
    BlendMode,
    LineCapStyle,
    PDFDocument,
    rgb as pdfRgb
  } from 'pdf-lib'
  import PdfPage from './PdfPage.svelte'
  import Icon from './Icon.svelte'
  import type { Bookmark, DocInfo, ReadingState, Stroke, Tool } from './types'
  import {
    ExportAnnotatedPDF,
    GetPDFURL,
    GetReadingState,
    LoadBookmarks,
    LoadStrokes,
    SaveBookmarks,
    SaveReadingState,
    SaveStrokes,
    SetPageCount
  } from '../../wailsjs/go/main/App.js'

  pdfjsLib.GlobalWorkerOptions.workerSrc = pdfWorkerUrl

  export let doc: DocInfo
  export let onClose: () => void = () => {}
  export let onDocUpdate: (doc: DocInfo) => void = () => {}

  type PageSize = { index: number; width: number; height: number }
  type OutlineItem = {
    id: string
    title: string
    page: number
    depth: number
  }
  type SidebarTab = 'outline' | 'bookmarks' | 'annotations'
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
  let sidebarOpen = false
  let sidebarTab: SidebarTab = 'outline'
  let outlineLoading = false
  let outlineItems: OutlineItem[] = []
  let bookmarks: Bookmark[] = []
  let bookmarksLoaded = false
  let exporting = false
  let toastMessage = ''
  let toastIsError = false
  let passwordNeeded = false
  let passwordInput = ''
  let passwordError = ''
  let suppressSave = false

  let scrollTimer: ReturnType<typeof setTimeout> | undefined
  let strokeTimer: ReturnType<typeof setTimeout> | undefined
  let resizeTimer: ReturnType<typeof setTimeout> | undefined
  let resumeTimer: ReturnType<typeof setTimeout> | undefined
  let bookmarkTimer: ReturnType<typeof setTimeout> | undefined
  let toastTimer: ReturnType<typeof setTimeout> | undefined
  let saveQueue: Promise<void> = Promise.resolve()
  let bookmarkSaveQueue: Promise<void> = Promise.resolve()
  let destroyed = false
  let strokesLoaded = false
  let resizeObserver: ResizeObserver | null = null

  let initialState: ReadingState = {
    docId: doc.id,
    page: 1,
    offset: 0,
    zoom: 1,
    furthestPage: 1,
    updatedAt: 0
  }
  let savedStrokes: Stroke[] = []
  let savedBookmarks: Bookmark[] = []
  let currentBookmark: Bookmark | null = null
  let annotationPages: { page: number; count: number }[] = []

  $: totalPages = pageSizes.length || doc.pageCount || 1
  $: contentWidth = maxPageWidth * fitScale * zoom
  $: currentBookmark = bookmarks.find((item) => item.page === currentPage) || null
  $: annotationPages = buildAnnotationPages(strokes)
  $: progress =
    totalPages > 0
      ? Math.min(100, (Math.max(furthestPage, currentPage) / totalPages) * 100)
      : 0

  onMount(() => {
    void init()
    scroller?.addEventListener('wheel', handleWheel, { passive: false })
    resizeObserver = new ResizeObserver(() => handleResize())
    if (scroller) resizeObserver.observe(scroller)
    window.addEventListener('keydown', handleKeydown)
    window.addEventListener('resize', handleResize)
    document.addEventListener('visibilitychange', handleVisibility)
    return () => {
      destroyed = true
      window.removeEventListener('keydown', handleKeydown)
      window.removeEventListener('resize', handleResize)
      document.removeEventListener('visibilitychange', handleVisibility)
      scroller?.removeEventListener('wheel', handleWheel)
      resizeObserver?.disconnect()
      if (scrollTimer) clearTimeout(scrollTimer)
      if (strokeTimer) clearTimeout(strokeTimer)
      if (resizeTimer) clearTimeout(resizeTimer)
      if (resumeTimer) clearTimeout(resumeTimer)
      if (bookmarkTimer) clearTimeout(bookmarkTimer)
      if (toastTimer) clearTimeout(toastTimer)
      void loadingTask?.destroy()
    }
  })

  onDestroy(() => {
    void loadingTask?.destroy()
  })

  async function init() {
    try {
      strokesLoaded = false
      bookmarksLoaded = false
      loading = true
      loadingText = '正在打开文档…'
      const [urlResult, stateResult, strokesResult, bookmarksResult] = await Promise.all([
        GetPDFURL(doc.id),
        GetReadingState(doc.id),
        LoadStrokes(doc.id),
        LoadBookmarks(doc.id)
      ])
      pdfUrl = String(urlResult || '')
      if (!pdfUrl) throw new Error('无法读取 PDF 地址')
      initialState = stateResult as unknown as ReadingState
      savedStrokes = (strokesResult as unknown as Stroke[]) || []
      savedBookmarks = (bookmarksResult as unknown as Bookmark[]) || []

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
    const firstPage = await loaded.getPage(1)
    const firstViewport = firstPage.getViewport({ scale: 1 })
    firstPage.cleanup()
    pageSizes = Array.from({ length: loaded.numPages }, (_, index) => ({
      index: index + 1,
      width: firstViewport.width,
      height: firstViewport.height
    }))
    maxPageWidth = firstViewport.width
    fitScale = computeFitScale()
    loading = false
    void loadOutline(loaded)
  }

  function finishOpen() {
    strokes = savedStrokes
    strokesLoaded = true
    bookmarks = [...savedBookmarks].sort((a, b) => a.page - b.page)
    bookmarksLoaded = true
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

  function computeFitScale() {
    const width = scroller?.clientWidth || window.innerWidth || 1000
    return Math.max(0.15, Math.max(320, width - 64) / Math.max(1, maxPageWidth))
  }

  function handlePageSize(index: number, width: number, height: number) {
    const current = pageSizes[index - 1]
    if (
      !current ||
      (Math.abs(current.width - width) < 0.5 &&
        Math.abs(current.height - height) < 0.5)
    ) {
      return
    }
    const position = computePosition()
    pageSizes = pageSizes.map((page) =>
      page.index === index ? { ...page, width, height } : page
    )
    maxPageWidth = Math.max(...pageSizes.map((page) => page.width), 1)
    fitScale = computeFitScale()
    void tick().then(() => {
      requestAnimationFrame(() => scrollToPosition(position))
    })
  }

  async function loadOutline(loaded: PDFDocumentProxy) {
    outlineLoading = true
    outlineItems = []
    try {
      const raw = await loaded.getOutline()
      if (!raw?.length) return
      const result: OutlineItem[] = []
      let sequence = 0

      const walk = async (nodes: any[], depth: number) => {
        for (const node of nodes) {
          const page = await resolveDestPage(loaded, node.dest)
          if (page) {
            result.push({
              id: `outline-${sequence++}`,
              title: String(node.title || '未命名'),
              page,
              depth
            })
          }
          if (Array.isArray(node.items) && node.items.length) {
            await walk(node.items, depth + 1)
          }
        }
      }

      await walk(raw, 0)
      outlineItems = result
    } catch (reason) {
      console.error('load outline', reason)
      outlineItems = []
    } finally {
      outlineLoading = false
    }
  }

  async function resolveDestPage(loaded: PDFDocumentProxy, dest: any) {
    try {
      let target = dest
      if (typeof target === 'string') {
        target = await loaded.getDestination(target)
      }
      if (!Array.isArray(target) || !target.length) return null
      const first = target[0]
      if (typeof first === 'number') {
        return clamp(Math.round(first) + 1, 1, loaded.numPages)
      }
      if (first) {
        const index = await loaded.getPageIndex(first)
        return clamp(index + 1, 1, loaded.numPages)
      }
    } catch (reason) {
      console.error('resolve outline destination', reason)
    }
    return null
  }

  function buildAnnotationPages(items: Stroke[]) {
    const counts = new Map<number, number>()
    for (const stroke of items) {
      counts.set(stroke.page, (counts.get(stroke.page) || 0) + 1)
    }
    return [...counts.entries()]
      .sort((a, b) => a[0] - b[0])
      .map(([page, count]) => ({ page, count }))
  }

  function toggleSidebar() {
    sidebarOpen = !sidebarOpen
    window.setTimeout(handleResize, 80)
  }

  function openSidebar(tab: SidebarTab) {
    sidebarTab = tab
    sidebarOpen = true
    window.setTimeout(handleResize, 80)
  }

  function toggleBookmark() {
    if (!pdf) return
    const existing = bookmarks.find((item) => item.page === currentPage)
    if (existing) {
      bookmarks = bookmarks.filter((item) => item.id !== existing.id)
      showToast(`已取消第 ${currentPage} 页书签`)
    } else {
      bookmarks = [
        ...bookmarks,
        {
          id: crypto.randomUUID(),
          page: currentPage,
          label: `第 ${currentPage} 页`,
          createdAt: Date.now()
        }
      ].sort((a, b) => a.page - b.page)
      showToast(`已收藏第 ${currentPage} 页`)
    }
    scheduleSaveBookmarks()
  }

  function removeBookmark(bookmarkID: string) {
    bookmarks = bookmarks.filter((item) => item.id !== bookmarkID)
    scheduleSaveBookmarks()
  }

  function scheduleSaveBookmarks() {
    if (!bookmarksLoaded) return
    if (bookmarkTimer) clearTimeout(bookmarkTimer)
    bookmarkTimer = setTimeout(() => {
      const snapshot = bookmarks
      bookmarkSaveQueue = bookmarkSaveQueue
        .then(() => SaveBookmarks(doc.id, snapshot as any))
        .then(() => undefined)
        .catch((reason) => {
          console.error('save bookmarks', reason)
        })
    }, 400)
  }

  async function flushBookmarks() {
    if (!bookmarksLoaded) return
    if (bookmarkTimer) clearTimeout(bookmarkTimer)
    const snapshot = bookmarks
    bookmarkSaveQueue = bookmarkSaveQueue
      .then(() => SaveBookmarks(doc.id, snapshot as any))
      .then(() => undefined)
      .catch((reason) => {
        console.error('save bookmarks', reason)
      })
    await bookmarkSaveQueue
  }

  async function exportAnnotatedPDF() {
    if (!pdf || exporting) return
    exporting = true
    showToast('正在生成带批注 PDF…')
    try {
      const response = await fetch(pdfUrl)
      if (!response.ok) throw new Error('无法读取原 PDF')
      const source = await response.arrayBuffer()
      const exported = await PDFDocument.load(source)

      const grouped = new Map<number, Stroke[]>()
      for (const stroke of strokes) {
        const group = grouped.get(stroke.page) || []
        group.push(stroke)
        grouped.set(stroke.page, group)
      }

      for (const [pageNumber, pageStrokes] of grouped) {
        if (pageNumber < 1 || pageNumber > exported.getPageCount()) continue
        const page = exported.getPage(pageNumber - 1)
        const { width, height } = page.getSize()
        for (const stroke of pageStrokes) {
          const [red, green, blue] = hexToRgb(stroke.color)
          const lineWidth = Math.max(0.5, stroke.width * width)
          const opacity = stroke.tool === 'highlighter' ? 0.32 : 1
          const blendMode =
            stroke.tool === 'highlighter' ? BlendMode.Multiply : BlendMode.Normal

          if (stroke.points.length === 1) {
            const point = stroke.points[0]
            page.drawCircle({
              x: point.x * width,
              y: height - point.y * height,
              size: lineWidth / 2,
              color: pdfRgb(red, green, blue),
              opacity,
              blendMode
            })
            continue
          }

          const path = stroke.points
            .map(
              (point, index) =>
                `${index === 0 ? 'M' : 'L'} ${(point.x * width).toFixed(2)} ${(
                  point.y * height
                ).toFixed(2)}`
            )
            .join(' ')
          page.drawSvgPath(path, {
            x: 0,
            y: height,
            borderColor: pdfRgb(red, green, blue),
            borderWidth: lineWidth,
            borderOpacity: opacity,
            borderLineCap: LineCapStyle.Round,
            blendMode
          })
        }
      }

      const bytes = await exported.save()
      const suggested = `${doc.name.replace(/\.pdf$/i, '')}-annotated.pdf`
      const savedPath = await ExportAnnotatedPDF(
        suggested,
        bytesToBase64(bytes)
      )
      if (savedPath) {
        showToast(`已导出：${savedPath}`)
      } else {
        showToast('已取消导出')
      }
    } catch (reason: any) {
      const message = readableError(reason)
      showToast(
        /encrypt/i.test(message)
          ? '加密 PDF 暂不支持导出，请先解除密码'
          : `导出失败：${message}`,
        true
      )
    } finally {
      exporting = false
    }
  }

  function bytesToBase64(bytes: Uint8Array) {
    let binary = ''
    const chunkSize = 0x4000
    for (let index = 0; index < bytes.length; index += chunkSize) {
      binary += String.fromCharCode(
        ...bytes.subarray(index, index + chunkSize)
      )
    }
    return btoa(binary)
  }

  function hexToRgb(value: string): [number, number, number] {
    const hex = value.replace('#', '')
    const full =
      hex.length === 3
        ? hex
            .split('')
            .map((character) => character + character)
            .join('')
        : hex
    const number = Number.parseInt(full, 16)
    return [
      ((number >> 16) & 255) / 255,
      ((number >> 8) & 255) / 255,
      (number & 255) / 255
    ]
  }

  function showToast(message: string, isError = false) {
    toastMessage = message
    toastIsError = isError
    if (toastTimer) clearTimeout(toastTimer)
    toastTimer = setTimeout(() => {
      toastMessage = ''
    }, isError ? 5200 : 3400)
  }

  function formatBookmarkTime(timestamp: number) {
    return new Date(timestamp).toLocaleDateString('zh-CN', {
      month: 'numeric',
      day: 'numeric'
    })
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
      void flushBookmarks()
    }
  }

  async function handleClose() {
    if (pdf) {
      savePosition()
      await flushStrokes()
      await flushBookmarks()
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
    if ((event.ctrlKey || event.metaKey) && key === 'd') {
      event.preventDefault()
      toggleBookmark()
      return
    }
    if ((event.ctrlKey || event.metaKey) && key === 'b') {
      event.preventDefault()
      toggleSidebar()
      return
    }
    if ((event.ctrlKey || event.metaKey) && key === 'e') {
      event.preventDefault()
      void exportAnnotatedPDF()
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
      class="icon-button"
      class:active={currentBookmark}
      title={currentBookmark ? '取消当前页书签（Ctrl+D）' : '收藏当前页（Ctrl+D）'}
      aria-label={currentBookmark ? '取消书签' : '添加书签'}
      on:click={toggleBookmark}
    >
      <Icon name={currentBookmark ? 'bookmark-filled' : 'bookmark'} size={18} />
    </button>

    <button
      class="icon-button"
      title="导出带批注 PDF（Ctrl+E）"
      aria-label="导出带批注 PDF"
      disabled={exporting}
      on:click={exportAnnotatedPDF}
    >
      {#if exporting}
        <span class="mini-spinner dark"></span>
      {:else}
        <Icon name="export" size={18} />
      {/if}
    </button>

    <button
      class="icon-button"
      class:active={sidebarOpen}
      title="目录、书签和批注（Ctrl+B）"
      aria-label="打开导航侧栏"
      on:click={toggleSidebar}
    >
      <Icon name="sidebar" size={18} />
    </button>

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

  <div class="reader-body">
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
                  pageHeight={page.height}
                  cssWidth={pageCssWidth}
                  cssHeight={pageCssHeight}
                  {mode}
                  {tool}
                  {color}
                  {penWidth}
                  {strokes}
                  onStroke={addStroke}
                  onErase={eraseStroke}
                  onPageSize={handlePageSize}
                />
              </div>
            {/each}
          </div>
        </div>
      {/if}
    </main>

    {#if sidebarOpen}
      <aside class="sidebar" aria-label="文档导航">
        <div class="sidebar-head">
          <div>
            <div class="sidebar-kicker">DOCUMENT</div>
            <div class="sidebar-title">导航</div>
          </div>
          <button
            class="icon-button small"
            title="关闭侧栏"
            aria-label="关闭侧栏"
            on:click={toggleSidebar}
          >
            <Icon name="close" size={16} />
          </button>
        </div>

        <div class="sidebar-tabs">
          <button
            class:active={sidebarTab === 'outline'}
            on:click={() => (sidebarTab = 'outline')}
          >
            目录
          </button>
          <button
            class:active={sidebarTab === 'bookmarks'}
            on:click={() => (sidebarTab = 'bookmarks')}
          >
            书签<span>{bookmarks.length}</span>
          </button>
          <button
            class:active={sidebarTab === 'annotations'}
            on:click={() => (sidebarTab = 'annotations')}
          >
            批注<span>{strokes.length}</span>
          </button>
        </div>

        <div class="sidebar-content">
          {#if sidebarTab === 'outline'}
            {#if outlineLoading}
              <div class="sidebar-empty">正在读取目录…</div>
            {:else if outlineItems.length === 0}
              <div class="sidebar-empty">这份 PDF 没有可用的目录</div>
            {:else}
              {#each outlineItems as item (item.id)}
                <button
                  class="outline-item"
                  style:padding-left={`${12 + item.depth * 16}px`}
                  title={item.title}
                  on:click={() => jumpToPage(item.page)}
                >
                  <span class="outline-title">{item.title}</span>
                  <span class="outline-page">{item.page}</span>
                </button>
              {/each}
            {/if}
          {:else if sidebarTab === 'bookmarks'}
            <button class="sidebar-action" on:click={toggleBookmark}>
              <Icon
                name={currentBookmark ? 'bookmark-filled' : 'bookmark'}
                size={16}
              />
              {currentBookmark ? '取消当前页书签' : `收藏第 ${currentPage} 页`}
            </button>
            {#if bookmarks.length === 0}
              <div class="sidebar-empty">还没有书签</div>
            {:else}
              {#each bookmarks as bookmark (bookmark.id)}
                <div class="bookmark-row">
                  <button
                    class="bookmark-main"
                    on:click={() => jumpToPage(bookmark.page)}
                  >
                    <span>{bookmark.label}</span>
                    <small>{formatBookmarkTime(bookmark.createdAt)}</small>
                  </button>
                  <button
                    class="row-delete"
                    title="删除书签"
                    aria-label="删除书签"
                    on:click={() => removeBookmark(bookmark.id)}
                  >
                    <Icon name="trash" size={14} />
                  </button>
                </div>
              {/each}
            {/if}
          {:else}
            {#if annotationPages.length === 0}
              <div class="sidebar-empty">还没有鼠标批注</div>
            {:else}
              {#each annotationPages as item (item.page)}
                <button class="annotation-row" on:click={() => jumpToPage(item.page)}>
                  <span class="annotation-page">第 {item.page} 页</span>
                  <span class="annotation-count">{item.count} 笔</span>
                </button>
              {/each}
            {/if}
          {/if}
        </div>
      </aside>
    {/if}
  </div>

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

  {#if toastMessage}
    <div class="export-toast" class:error={toastIsError}>{toastMessage}</div>
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

  .icon-button.active {
    background: #20242b;
    color: #fff;
  }

  .icon-button:disabled {
    opacity: 0.45;
    cursor: default;
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

  .reader-body {
    flex: 1;
    min-height: 0;
    display: flex;
    overflow: hidden;
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

  .sidebar {
    width: 316px;
    flex: 0 0 316px;
    display: flex;
    flex-direction: column;
    min-height: 0;
    background: #fbfaf7;
    border-left: 1px solid rgba(32, 36, 43, 0.1);
    box-shadow: -10px 0 30px rgba(32, 36, 43, 0.04);
    z-index: 15;
  }

  .sidebar-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    padding: 18px 14px 12px 18px;
  }

  .sidebar-kicker {
    color: #b0a89b;
    font-size: 8px;
    font-weight: 750;
    letter-spacing: 0.16em;
  }

  .sidebar-title {
    margin-top: 4px;
    font-size: 15px;
    font-weight: 700;
    letter-spacing: -0.01em;
  }

  .sidebar-tabs {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 4px;
    padding: 0 12px 10px;
  }

  .sidebar-tabs button {
    height: 32px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 4px;
    border: 0;
    border-radius: 8px;
    background: transparent;
    color: #827d74;
    font-size: 10px;
    font-weight: 650;
    cursor: pointer;
  }

  .sidebar-tabs button:hover {
    background: rgba(32, 36, 43, 0.05);
  }

  .sidebar-tabs button.active {
    background: #20242b;
    color: #fff;
  }

  .sidebar-tabs button span {
    min-width: 15px;
    padding: 1px 4px;
    border-radius: 999px;
    background: rgba(32, 36, 43, 0.08);
    font-size: 8px;
    text-align: center;
  }

  .sidebar-tabs button.active span {
    background: rgba(255, 255, 255, 0.16);
  }

  .sidebar-content {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    padding: 0 10px 18px;
  }

  .sidebar-empty {
    padding: 28px 14px;
    color: #a29b90;
    font-size: 11px;
    line-height: 1.7;
    text-align: center;
  }

  .outline-item {
    width: 100%;
    min-height: 34px;
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 7px 10px;
    border: 0;
    border-radius: 7px;
    background: transparent;
    color: #4c4841;
    text-align: left;
    cursor: pointer;
  }

  .outline-item:hover {
    background: rgba(32, 36, 43, 0.055);
  }

  .outline-title {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 11px;
    line-height: 1.45;
  }

  .outline-page {
    flex: 0 0 auto;
    color: #aaa398;
    font-size: 9px;
    font-variant-numeric: tabular-nums;
  }

  .sidebar-action {
    width: 100%;
    min-height: 36px;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 7px;
    margin-bottom: 8px;
    border: 1px dashed rgba(32, 36, 43, 0.18);
    border-radius: 9px;
    background: transparent;
    color: #5f5a52;
    font-size: 10px;
    font-weight: 650;
    cursor: pointer;
  }

  .sidebar-action:hover {
    background: #fff;
  }

  .bookmark-row {
    display: flex;
    align-items: stretch;
    border-bottom: 1px solid rgba(32, 36, 43, 0.06);
  }

  .bookmark-main {
    flex: 1;
    min-width: 0;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    padding: 10px 8px 10px 10px;
    border: 0;
    background: transparent;
    color: #4d4942;
    text-align: left;
    cursor: pointer;
  }

  .bookmark-main:hover {
    background: rgba(32, 36, 43, 0.04);
  }

  .bookmark-main span {
    font-size: 11px;
    font-weight: 600;
  }

  .bookmark-main small {
    color: #aaa398;
    font-size: 9px;
  }

  .row-delete {
    width: 28px;
    display: grid;
    place-items: center;
    border: 0;
    border-radius: 7px;
    background: transparent;
    color: #b2aba0;
    cursor: pointer;
  }

  .row-delete:hover {
    background: #fbe9e5;
    color: #b23a2a;
  }

  .annotation-row {
    width: 100%;
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 11px 10px;
    border: 0;
    border-bottom: 1px solid rgba(32, 36, 43, 0.06);
    background: transparent;
    color: #4d4942;
    cursor: pointer;
  }

  .annotation-row:hover {
    background: rgba(32, 36, 43, 0.04);
  }

  .annotation-page {
    font-size: 11px;
    font-weight: 600;
  }

  .annotation-count {
    color: #aaa398;
    font-size: 9px;
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

  .export-toast {
    position: absolute;
    top: 70px;
    left: 50%;
    transform: translateX(-50%);
    max-width: min(560px, calc(100vw - 80px));
    padding: 9px 15px;
    border-radius: 10px;
    background: rgba(32, 36, 43, 0.92);
    color: #fff;
    font-size: 11px;
    line-height: 1.5;
    text-align: center;
    box-shadow: 0 8px 26px rgba(20, 22, 26, 0.18);
    z-index: 26;
    word-break: break-word;
  }

  .export-toast.error {
    background: rgba(150, 48, 35, 0.95);
  }

  .mini-spinner {
    width: 14px;
    height: 14px;
    border: 2px solid rgba(32, 36, 43, 0.15);
    border-top-color: #c8553d;
    border-radius: 50%;
    animation: spin 700ms linear infinite;
  }

  .mini-spinner.dark {
    border-color: rgba(32, 36, 43, 0.15);
    border-top-color: #c8553d;
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
