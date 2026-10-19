<script lang="ts">
  import { onDestroy, onMount } from 'svelte'
  import type { PDFDocumentProxy, RenderTask } from 'pdfjs-dist'
  import type { Point, Stroke, Tool } from './types'

  export let pdf: PDFDocumentProxy
  export let pageIndex: number
  export let pageWidth: number
  export let cssWidth: number
  export let cssHeight: number
  export let mode: 'read' | 'annotate' = 'read'
  export let tool: Tool = 'pen'
  export let color = '#1f2937'
  export let penWidth = 2
  export let strokes: Stroke[] = []
  export let onStroke: (stroke: Stroke) => void = () => {}
  export let onErase: (strokeId: string) => void = () => {}
  export let onVisible: (page: number) => void = () => {}

  let host: HTMLDivElement
  let pdfCanvas: HTMLCanvasElement
  let inkCanvas: HTMLCanvasElement
  let observer: IntersectionObserver | null = null
  let active = false
  let renderedWidth = 0
  let renderedHeight = 0
  let renderTask: RenderTask | null = null
  let renderSequence = 0
  let dpr = Math.min(window.devicePixelRatio || 1, 2)
  let pageStrokes: Stroke[] = []

  let drawing = false
  let pointerId = -1
  let currentPoints: Point[] = []
  let lastPoint: Point | null = null
  let activeTool: Tool = 'pen'
  let activeColor = '#1f2937'
  let activeWidth = 2
  let activeWidthNorm = 0.002

  $: dpr = Math.min(window.devicePixelRatio || 1, 2)
  $: pageStrokes = strokes.filter((stroke) => stroke.page === pageIndex)
  $: if (active && pdf && inkCanvas) {
    sizeInkCanvas()
    if (renderedWidth !== cssWidth || renderedHeight !== cssHeight) {
      void renderPdf()
    }
  }

  onMount(() => {
    observer = new IntersectionObserver(
      (entries) => {
        const entry = entries[0]
        if (!entry) return
        if (entry.isIntersecting) {
          active = true
          onVisible(pageIndex)
        } else {
          active = false
          releaseCanvases()
        }
      },
      { root: null, rootMargin: '900px 0px', threshold: 0.01 }
    )
    if (host) observer.observe(host)
    return () => observer?.disconnect()
  })

  onDestroy(() => {
    observer?.disconnect()
    releaseCanvases()
  })

  function releaseCanvases() {
    renderSequence += 1
    if (renderTask) {
      renderTask.cancel()
      renderTask = null
    }
    if (pdfCanvas) {
      pdfCanvas.width = 0
      pdfCanvas.height = 0
    }
    if (inkCanvas) {
      inkCanvas.width = 0
      inkCanvas.height = 0
    }
    renderedWidth = 0
    renderedHeight = 0
  }

  async function renderPdf() {
    if (!active || !pdf || !pdfCanvas) return
    const sequence = ++renderSequence
    const targetWidth = cssWidth
    const targetHeight = cssHeight

    if (renderTask) {
      renderTask.cancel()
      renderTask = null
    }

    let page
    try {
      page = await pdf.getPage(pageIndex)
    } catch (error) {
      console.error('getPage failed', pageIndex, error)
      return
    }
    if (sequence !== renderSequence || !active) {
      page.cleanup()
      return
    }

    const viewport = page.getViewport({
      scale: (targetWidth / pageWidth) * dpr
    })
    const canvas = pdfCanvas
    canvas.width = Math.max(1, Math.floor(viewport.width))
    canvas.height = Math.max(1, Math.floor(viewport.height))
    canvas.style.width = `${targetWidth}px`
    canvas.style.height = `${targetHeight}px`

    const context = canvas.getContext('2d', { alpha: false })
    if (!context) return
    context.fillStyle = '#ffffff'
    context.fillRect(0, 0, canvas.width, canvas.height)

    const task = page.render({ canvasContext: context, canvas, viewport })
    renderTask = task
    try {
      await task.promise
    } catch (error: any) {
      if (error?.name !== 'RenderingCancelledException') {
        console.error('render failed', pageIndex, error)
      }
    } finally {
      if (renderTask === task) renderTask = null
      page.cleanup()
    }

    if (sequence !== renderSequence || !active) return
    renderedWidth = targetWidth
    renderedHeight = targetHeight
    redrawInk(pageStrokes)
  }

  function sizeInkCanvas() {
    if (!inkCanvas) return
    const width = Math.max(1, Math.round(cssWidth * dpr))
    const height = Math.max(1, Math.round(cssHeight * dpr))
    if (inkCanvas.width !== width) inkCanvas.width = width
    if (inkCanvas.height !== height) inkCanvas.height = height
    inkCanvas.style.width = `${cssWidth}px`
    inkCanvas.style.height = `${cssHeight}px`
    redrawInk(pageStrokes)
  }

  function redrawInk(items: Stroke[]) {
    if (!inkCanvas || inkCanvas.width === 0 || inkCanvas.height === 0) return
    const context = inkCanvas.getContext('2d')
    if (!context) return
    context.setTransform(dpr, 0, 0, dpr, 0, 0)
    context.clearRect(0, 0, cssWidth, cssHeight)
    for (const stroke of items) {
      drawStroke(context, stroke)
    }
  }

  function drawStroke(context: CanvasRenderingContext2D, stroke: Stroke) {
    const points = stroke.points
    if (!points.length) return
    const width = Math.max(0.7, stroke.width * cssWidth)
    context.save()
    context.lineCap = 'round'
    context.lineJoin = 'round'
    context.strokeStyle = stroke.color
    context.fillStyle = stroke.color
    context.globalAlpha = stroke.tool === 'highlighter' ? 0.32 : 1
    context.globalCompositeOperation =
      stroke.tool === 'highlighter' ? 'multiply' : 'source-over'
    context.lineWidth = width

    if (points.length === 1) {
      context.beginPath()
      context.arc(
        points[0].x * cssWidth,
        points[0].y * cssHeight,
        Math.max(0.6, width / 2),
        0,
        Math.PI * 2
      )
      context.fill()
      context.restore()
      return
    }

    context.beginPath()
    context.moveTo(points[0].x * cssWidth, points[0].y * cssHeight)
    for (let index = 1; index < points.length - 1; index += 1) {
      const point = points[index]
      const next = points[index + 1]
      const middleX = ((point.x + next.x) / 2) * cssWidth
      const middleY = ((point.y + next.y) / 2) * cssHeight
      context.quadraticCurveTo(
        point.x * cssWidth,
        point.y * cssHeight,
        middleX,
        middleY
      )
    }
    const last = points[points.length - 1]
    context.lineTo(last.x * cssWidth, last.y * cssHeight)
    context.stroke()
    context.restore()
  }

  function drawSegment(from: Point, to: Point) {
    const context = inkCanvas.getContext('2d')
    if (!context) return
    context.save()
    context.lineCap = 'round'
    context.lineJoin = 'round'
    context.strokeStyle = activeColor
    context.globalAlpha = activeTool === 'highlighter' ? 0.32 : 1
    context.globalCompositeOperation =
      activeTool === 'highlighter' ? 'multiply' : 'source-over'
    context.lineWidth = Math.max(0.7, activeWidth)
    context.beginPath()
    context.moveTo(from.x * cssWidth, from.y * cssHeight)
    context.lineTo(to.x * cssWidth, to.y * cssHeight)
    context.stroke()
    context.restore()
  }

  function drawDot(point: Point) {
    const context = inkCanvas.getContext('2d')
    if (!context) return
    context.save()
    context.fillStyle = activeColor
    context.globalAlpha = activeTool === 'highlighter' ? 0.32 : 1
    context.globalCompositeOperation =
      activeTool === 'highlighter' ? 'multiply' : 'source-over'
    context.beginPath()
    context.arc(
      point.x * cssWidth,
      point.y * cssHeight,
      Math.max(0.6, activeWidth / 2),
      0,
      Math.PI * 2
    )
    context.fill()
    context.restore()
  }

  function pointFromEvent(event: PointerEvent): Point {
    const rect = inkCanvas.getBoundingClientRect()
    return {
      x: clamp01((event.clientX - rect.left) / Math.max(1, rect.width)),
      y: clamp01((event.clientY - rect.top) / Math.max(1, rect.height))
    }
  }

  function handlePointerDown(event: PointerEvent) {
    if (mode !== 'annotate' || event.button !== 0) return
    event.preventDefault()
    const point = pointFromEvent(event)

    if (tool === 'eraser') {
      eraseAt(point)
      return
    }

    activeTool = tool
    activeColor = color
    activeWidth =
      tool === 'highlighter' ? Math.max(12, penWidth * 4.5) : Math.max(1, penWidth)
    activeWidthNorm = activeWidth / Math.max(1, cssWidth)

    drawing = true
    pointerId = event.pointerId
    currentPoints = [point]
    lastPoint = point
    inkCanvas.setPointerCapture(event.pointerId)
    drawDot(point)
  }

  function handlePointerMove(event: PointerEvent) {
    if (!drawing || event.pointerId !== pointerId) return
    event.preventDefault()
    const coalesced = (event as PointerEvent & {
      getCoalescedEvents?: () => PointerEvent[]
    }).getCoalescedEvents?.()
    const events = coalesced && coalesced.length ? coalesced : [event]

    for (const item of events) {
      const point = pointFromEvent(item)
      if (!lastPoint) {
        currentPoints.push(point)
        lastPoint = point
        drawDot(point)
        continue
      }
      const dx = point.x - lastPoint.x
      const dy = point.y - lastPoint.y
      if (dx * dx + dy * dy < 0.0000004) continue
      currentPoints.push(point)
      drawSegment(lastPoint, point)
      lastPoint = point
    }
  }

  function finishStroke(event?: PointerEvent) {
    if (!drawing) return
    drawing = false
    if (event && inkCanvas.hasPointerCapture(event.pointerId)) {
      inkCanvas.releasePointerCapture(event.pointerId)
    }
    if (!currentPoints.length) {
      currentPoints = []
      lastPoint = null
      return
    }

    const stroke: Stroke = {
      id: crypto.randomUUID(),
      page: pageIndex,
      tool: activeTool,
      color: activeColor,
      width: activeWidthNorm,
      points: currentPoints.map((point) => ({ ...point })),
      createdAt: Date.now()
    }
    currentPoints = []
    lastPoint = null
    onStroke(stroke)
    redrawInk([...pageStrokes, stroke])
  }

  function eraseAt(point: Point) {
    for (let index = pageStrokes.length - 1; index >= 0; index -= 1) {
      const stroke = pageStrokes[index]
      const width = Math.max(0.7, stroke.width * cssWidth)
      const threshold = Math.max(10, width * 0.65 + 4)
      if (isStrokeHit(stroke, point, threshold)) {
        onErase(stroke.id)
        redrawInk(pageStrokes.filter((item) => item.id !== stroke.id))
        return
      }
    }
  }

  function isStrokeHit(stroke: Stroke, point: Point, threshold: number) {
    const px = point.x * cssWidth
    const py = point.y * cssHeight
    const points = stroke.points.map((item) => ({
      x: item.x * cssWidth,
      y: item.y * cssHeight
    }))
    if (points.length === 1) {
      return Math.hypot(px - points[0].x, py - points[0].y) <= threshold
    }
    for (let index = 1; index < points.length; index += 1) {
      if (
        distanceToSegment(px, py, points[index - 1], points[index]) <= threshold
      ) {
        return true
      }
    }
    return false
  }

  function distanceToSegment(
    px: number,
    py: number,
    a: { x: number; y: number },
    b: { x: number; y: number }
  ) {
    const dx = b.x - a.x
    const dy = b.y - a.y
    const lengthSquared = dx * dx + dy * dy
    if (lengthSquared === 0) return Math.hypot(px - a.x, py - a.y)
    let t = ((px - a.x) * dx + (py - a.y) * dy) / lengthSquared
    t = Math.max(0, Math.min(1, t))
    return Math.hypot(px - (a.x + t * dx), py - (a.y + t * dy))
  }

  function clamp01(value: number) {
    if (value < 0) return 0
    if (value > 1) return 1
    return value
  }
</script>

<div
  class="pdf-page"
  class:annotate={mode === 'annotate'}
  class:eraser={mode === 'annotate' && tool === 'eraser'}
  style:width={`${cssWidth}px`}
  style:height={`${cssHeight}px`}
  bind:this={host}
>
  <canvas class="pdf-canvas" bind:this={pdfCanvas} aria-label={`第 ${pageIndex} 页`}></canvas>
  <canvas
    class="ink-canvas"
    bind:this={inkCanvas}
    on:pointerdown={handlePointerDown}
    on:pointermove={handlePointerMove}
    on:pointerup={finishStroke}
    on:pointercancel={finishStroke}
    on:contextmenu|preventDefault={() => {}}
  ></canvas>
</div>

<style>
  .pdf-page {
    position: relative;
    background: #fff;
    border-radius: 3px;
    box-shadow:
      0 1px 2px rgba(31, 35, 40, 0.06),
      0 8px 28px rgba(31, 35, 40, 0.1);
    overflow: hidden;
    user-select: none;
  }

  .pdf-canvas,
  .ink-canvas {
    position: absolute;
    inset: 0;
    display: block;
  }

  .ink-canvas {
    z-index: 2;
    touch-action: none;
  }

  .pdf-page.annotate .ink-canvas {
    cursor: crosshair;
  }

  .pdf-page.annotate.eraser .ink-canvas {
    cursor: cell;
  }
</style>
