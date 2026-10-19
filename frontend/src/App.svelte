<script lang="ts">
  import { onMount } from 'svelte'
  import Home from './lib/Home.svelte'
  import Reader from './lib/Reader.svelte'
  import type { DocInfo } from './lib/types'
  import { EventsOn } from '../wailsjs/runtime/runtime.js'
  import {
    GetRecentDocs,
    OpenPDF,
    OpenPath,
    OpenRecent,
    RemoveRecent,
    TakeStartupDoc
  } from '../wailsjs/go/main/App.js'

  let readerDoc: DocInfo | null = null
  let recent: DocInfo[] = []
  let opening = false
  let error = ''

  onMount(() => {
    void boot()
    const cancel = EventsOn('file-drop', (paths: string[]) => {
      const pdfPath = (paths || []).find((path) =>
        path.toLowerCase().endsWith('.pdf')
      )
      if (pdfPath) void openDocumentPath(pdfPath)
    })
    return () => cancel?.()
  })

  async function boot() {
    await refreshRecent()
    try {
      const result = await TakeStartupDoc()
      const document = result as unknown as DocInfo
      if (document?.id) readerDoc = document
    } catch (reason) {
      console.error('take startup document', reason)
    }
  }

  async function refreshRecent() {
    try {
      const docs = await GetRecentDocs()
      recent = (docs as unknown as DocInfo[]) || []
    } catch (reason) {
      console.error('load recent documents', reason)
    }
  }

  async function openPicker() {
    if (opening) return
    opening = true
    error = ''
    try {
      const result = await OpenPDF()
      const document = result as unknown as DocInfo
      if (document?.id) {
        readerDoc = document
      }
      await refreshRecent()
    } catch (reason) {
      error = readableError(reason)
    } finally {
      opening = false
    }
  }

  async function openRecent(document: DocInfo) {
    if (opening) return
    opening = true
    error = ''
    try {
      const result = await OpenRecent(document.id)
      const opened = result as unknown as DocInfo
      if (opened?.id) readerDoc = opened
      await refreshRecent()
    } catch (reason) {
      error = readableError(reason)
    } finally {
      opening = false
    }
  }

  async function openDocumentPath(path: string) {
    if (opening) return
    opening = true
    error = ''
    try {
      const result = await OpenPath(path)
      const opened = result as unknown as DocInfo
      if (opened?.id) readerDoc = opened
      await refreshRecent()
    } catch (reason) {
      error = readableError(reason)
    } finally {
      opening = false
    }
  }

  async function removeRecent(document: DocInfo) {
    try {
      await RemoveRecent(document.id)
      await refreshRecent()
    } catch (reason) {
      error = readableError(reason)
    }
  }

  function closeReader() {
    readerDoc = null
    error = ''
    void refreshRecent()
  }

  function readableError(reason: any) {
    if (reason instanceof Error) return reason.message
    if (typeof reason === 'string') return reason
    if (reason?.message) return String(reason.message)
    return '操作失败，请重试'
  }
</script>

{#if readerDoc}
  <Reader
    doc={readerDoc}
    onClose={closeReader}
    onDocUpdate={(document) => (readerDoc = document)}
  />
{:else}
  <Home
    {recent}
    {opening}
    {error}
    onOpen={openPicker}
    onOpenRecent={openRecent}
    onRemoveRecent={removeRecent}
  />
{/if}
