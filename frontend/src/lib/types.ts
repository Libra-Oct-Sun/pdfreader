export interface DocInfo {
  id: string
  name: string
  path: string
  pageCount: number
  lastPage: number
  offset: number
  zoom: number
  furthestPage: number
  updatedAt: number
  exists: boolean
}

export interface ReadingState {
  docId: string
  page: number
  offset: number
  zoom: number
  furthestPage: number
  updatedAt: number
}

export interface Point {
  x: number
  y: number
}

export type Tool = 'pen' | 'highlighter' | 'eraser'

export interface Stroke {
  id: string
  page: number
  tool: Tool
  color: string
  width: number
  points: Point[]
  createdAt: number
}
