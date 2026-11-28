/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { api } from '@/lib/api'

import type { ApiResponse, TicketListData, TicketDetailData } from './types'

// ============================================================================
// User side
// ============================================================================

export async function getUserTickets(
  page = 1,
  pageSize = 20
): Promise<ApiResponse<TicketListData>> {
  const res = await api.get(
    `/api/ticket/?page=${page}&page_size=${pageSize}`
  )
  return res.data
}

export async function createTicket(payload: {
  title: string
  content: string
}): Promise<ApiResponse<{ id: number }>> {
  const res = await api.post('/api/ticket/', payload)
  return res.data
}

export async function getTicketDetail(
  id: number
): Promise<ApiResponse<TicketDetailData>> {
  const res = await api.get(`/api/ticket/${id}`)
  return res.data
}

export async function replyTicket(
  id: number,
  content: string
): Promise<ApiResponse> {
  const res = await api.post(`/api/ticket/${id}/reply`, { content })
  return res.data
}

export async function closeTicket(id: number): Promise<ApiResponse> {
  const res = await api.post(`/api/ticket/${id}/close`)
  return res.data
}

/**
 * Unread ticket counts for sidebar badges: `user_count` for the viewer's own
 * tickets, `admin_count` for tickets awaiting admin attention (admins only).
 */
export async function getUnreadCount(): Promise<
  ApiResponse<{ user_count: number; admin_count: number }>
> {
  const res = await api.get('/api/ticket/unread-count')
  return res.data
}

// ============================================================================
// Admin side
// ============================================================================

export async function adminGetTickets(params: {
  page?: number
  page_size?: number
  status?: string
  keyword?: string
}): Promise<ApiResponse<TicketListData>> {
  const q = new URLSearchParams()
  q.set('page', String(params.page ?? 1))
  q.set('page_size', String(params.page_size ?? 20))
  if (params.status) q.set('status', params.status)
  if (params.keyword) q.set('keyword', params.keyword)
  const res = await api.get(`/api/ticket/admin/?${q.toString()}`)
  return res.data
}

export async function adminGetTicketDetail(
  id: number
): Promise<ApiResponse<TicketDetailData>> {
  const res = await api.get(`/api/ticket/admin/${id}`)
  return res.data
}

export async function adminReplyTicket(
  id: number,
  content: string
): Promise<ApiResponse> {
  const res = await api.post(`/api/ticket/admin/${id}/reply`, { content })
  return res.data
}

export async function adminCloseTicket(id: number): Promise<ApiResponse> {
  const res = await api.post(`/api/ticket/admin/${id}/close`)
  return res.data
}

// ============================================================================
// Image upload
// ============================================================================

export async function uploadTicketImage(
  file: File
): Promise<ApiResponse<{ url: string }>> {
  const formData = new FormData()
  formData.append('image', file)
  const res = await api.post('/api/ticket-image/', formData, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
  return res.data
}

/** Fetch a ticket image through the auth interceptor and return a blob URL
 *  the <img> tag can render without leaking credentials. Caller must revoke. */
export async function fetchTicketImageBlob(url: string): Promise<string> {
  const res = await api.get(url, { responseType: 'blob' })
  return URL.createObjectURL(res.data as Blob)
}
