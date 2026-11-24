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
import dayjs from 'dayjs'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { HtmlContent } from '@/components/html-content'
import { cn } from '@/lib/utils'

import { fetchTicketImageBlob } from '../api'
import type { TicketMessage } from '../types'

const TICKET_IMAGE_PREFIX = '/api/ticket-image/'

interface TicketMessageBubbleProps {
  message: TicketMessage
  isOwn: boolean
}

/**
 * Remove the src of every ticket image so the browser never fires an
 * unauthenticated request. Returns the stripped markup plus the images whose
 * blob still has to be fetched.
 */
function stripTicketImageSrcs(content: string): {
  html: string
  pending: { img: HTMLImageElement; src: string }[]
} {
  if (!content.includes(TICKET_IMAGE_PREFIX)) {
    return { html: content, pending: [] }
  }
  const template = document.createElement('template')
  template.innerHTML = content
  const pending: { img: HTMLImageElement; src: string }[] = []
  template.content.querySelectorAll('img').forEach((img) => {
    const src = img.getAttribute('src') ?? ''
    if (!src.startsWith(TICKET_IMAGE_PREFIX)) return
    img.removeAttribute('src')
    pending.push({ img, src })
  })
  return { html: template.innerHTML, pending }
}

/**
 * Ticket images live behind an authenticated route, but a bare <img> request
 * carries no Authorization header — the browser gets 401 and shows a broken
 * image. We therefore load them through the axios interceptor as blob URLs.
 *
 * Two details matter:
 *  - The original src is removed before the HTML reaches DOMPurify, so the
 *    browser never fires a doomed request (no 401 noise, no broken flash).
 *  - blob: must survive sanitization, so the renderer opts into allowBlobUrls.
 */
function useTicketImageContent(content: string): string {
  const [resolved, setResolved] = useState(() => stripTicketImageSrcs(content).html)
  const blobUrlsRef = useRef<string[]>([])

  useEffect(() => {
    if (!content.includes(TICKET_IMAGE_PREFIX)) {
      setResolved(content)
      return
    }

    let cancelled = false
    const template = document.createElement('template')
    template.innerHTML = content

    const pending: { img: HTMLImageElement; src: string }[] = []
    template.content.querySelectorAll('img').forEach((img) => {
      const src = img.getAttribute('src') ?? ''
      if (!src.startsWith(TICKET_IMAGE_PREFIX)) return
      img.removeAttribute('src')
      pending.push({ img, src })
    })

    // Render the stripped markup right away; the bubbles keep their text and
    // the images fill in as their blobs resolve.
    setResolved(template.innerHTML)

    if (pending.length === 0) return

    const tasks = pending.map(({ img, src }) =>
      fetchTicketImageBlob(src)
        .then((blobUrl) => {
          if (cancelled) return
          blobUrlsRef.current.push(blobUrl)
          img.src = blobUrl
        })
        .catch(() => {
          img.alt = '[image]'
        })
    )

    void Promise.all(tasks).then(() => {
      if (!cancelled) {
        setResolved(template.innerHTML)
      }
    })

    return () => {
      cancelled = true
    }
  }, [content])

  return resolved
}

export function TicketMessageBubble({
  message,
  isOwn,
}: TicketMessageBubbleProps) {
  const { t } = useTranslation()
  const content = useTicketImageContent(message.content)

  let senderLabel: string
  if (isOwn) {
    senderLabel = t('You')
  } else if (message.sender_role === 'admin') {
    senderLabel = t('Support')
  } else {
    senderLabel = message.sender_name || `#${message.sender_id}`
  }

  return (
    <div
      className={cn(
        'flex flex-col gap-1',
        isOwn ? 'items-end' : 'items-start'
      )}
    >
      <div className='text-muted-foreground flex items-center gap-2 text-xs'>
        <span className='font-medium'>{senderLabel}</span>
        <span>{dayjs.unix(message.created_at).format('YYYY-MM-DD HH:mm')}</span>
      </div>
      <div
        className={cn(
          'max-w-[85%] rounded-lg px-3 py-2 text-sm break-words [&_img]:my-1 [&_img]:max-w-full [&_img]:rounded-md [&_img]:block',
          isOwn ? 'bg-primary text-primary-foreground' : 'bg-muted'
        )}
      >
        <HtmlContent content={content} allowBlobUrls />
      </div>
    </div>
  )
}
