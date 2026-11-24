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
import { useEffect, useLayoutEffect, useRef, type RefObject } from 'react'

/** Distance (px) from the bottom that still counts as "pinned". */
const PIN_THRESHOLD = 96

/**
 * Keep a scroll container pinned to the bottom while new content arrives.
 *
 * - Jumps to the bottom when `dep` changes (initial load, new message,
 *   finished fetching) — the layout effect runs after the DOM is committed.
 * - Stays pinned while content grows later (images/blobs decoding change the
 *   height): a ResizeObserver re-scrolls, but only if the user has not
 *   scrolled up to read older messages.
 */
export function useStickToBottom(
  containerRef: RefObject<HTMLDivElement | null>,
  dep: unknown
) {
  const shouldStickRef = useRef(true)

  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    const handleScroll = () => {
      shouldStickRef.current =
        el.scrollHeight - el.scrollTop - el.clientHeight <= PIN_THRESHOLD
    }
    el.addEventListener('scroll', handleScroll, { passive: true })
    return () => el.removeEventListener('scroll', handleScroll)
  }, [containerRef])

  useLayoutEffect(() => {
    const el = containerRef.current
    if (!el) return
    shouldStickRef.current = true
    const scroll = () => {
      if (shouldStickRef.current) {
        el.scrollTop = el.scrollHeight
      }
    }
    scroll()
    // Second pass after the browser has laid out this commit's content.
    const frame = requestAnimationFrame(scroll)
    return () => cancelAnimationFrame(frame)
  }, [containerRef, dep])

  useEffect(() => {
    const el = containerRef.current
    const target = el?.firstElementChild
    if (!el || !target) return
    const observer = new ResizeObserver(() => {
      if (shouldStickRef.current) {
        el.scrollTop = el.scrollHeight
      }
    })
    observer.observe(target)
    return () => observer.disconnect()
  }, [containerRef, dep])
}
