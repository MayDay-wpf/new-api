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
import { Link, useParams } from '@tanstack/react-router'
import { ArrowLeft, Lock } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { SectionPageLayout } from '@/components/layout'
import { Skeleton } from '@/components/ui/skeleton'
import { useAuthStore } from '@/stores/auth-store'

import { closeTicket, getTicketDetail, replyTicket } from './api'
import { useInvalidateTicketUnread } from './hooks/use-ticket-badge'
import { useStickToBottom } from './hooks/use-stick-to-bottom'
import { TicketEditor } from './components/ticket-editor'
import { TicketMessageBubble } from './components/ticket-message-bubble'
import type { Ticket, TicketMessage } from './types'

export function TicketDetail() {
  const { t } = useTranslation()
  const { ticketId } = useParams({ strict: false })
  const id = Number(ticketId)
  const { auth } = useAuthStore()
  const invalidateUnread = useInvalidateTicketUnread()

  const [ticket, setTicket] = useState<Ticket | null>(null)
  const [messages, setMessages] = useState<TicketMessage[]>([])
  const [loading, setLoading] = useState(true)
  const [replyContent, setReplyContent] = useState('')
  const [sending, setSending] = useState(false)
  const [closing, setClosing] = useState(false)
  const [closeConfirm, setCloseConfirm] = useState(false)
  const scrollRef = useRef<HTMLDivElement>(null)

  const load = useCallback(async () => {
    if (!Number.isInteger(id)) return
    setLoading(true)
    try {
      const res = await getTicketDetail(id)
      if (res.success && res.data) {
        setTicket(res.data.ticket)
        setMessages(res.data.messages ?? [])
        invalidateUnread()
      }
    } finally {
      setLoading(false)
    }
  }, [id, invalidateUnread])

  useEffect(() => {
    void load()
  }, [load])

  // Open at the newest message and stay pinned after sending.
  useStickToBottom(scrollRef, messages.length)

  const handleReply = async () => {
    const content = replyContent.trim()
    if (!content) return
    setSending(true)
    try {
      const res = await replyTicket(id, content)
      if (res.success) {
        setReplyContent('')
        void load()
      } else {
        toast.error(res.message || t('Failed to send reply'))
      }
    } finally {
      setSending(false)
    }
  }

  const handleClose = async () => {
    setClosing(true)
    try {
      const res = await closeTicket(id)
      if (res.success) {
        toast.success(t('Ticket closed'))
        setCloseConfirm(false)
        void load()
      } else {
        toast.error(res.message || t('Failed to close ticket'))
      }
    } finally {
      setClosing(false)
    }
  }

  const isOpen = ticket?.status === 'open'

  return (
    <>
      <SectionPageLayout fixedContent>
        <SectionPageLayout.Title>
          <div className='flex items-center gap-3'>
            <Link to='/tickets'>
              <Button variant='ghost' size='icon-sm'>
                <ArrowLeft className='h-4 w-4' />
              </Button>
            </Link>
            <span className='truncate'>{ticket?.title ?? `#${ticketId}`}</span>
            {ticket && (
              <Badge
                variant={isOpen ? 'default' : 'secondary'}
                className='ml-2 shrink-0'
              >
                {isOpen ? t('Open') : t('Closed')}
              </Badge>
            )}
          </div>
        </SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          {isOpen && (
            <Button
              variant='outline'
              size='sm'
              onClick={() => setCloseConfirm(true)}
            >
              <Lock className='mr-1 h-4 w-4' />
              {t('Close Ticket')}
            </Button>
          )}
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='mx-auto flex h-full min-h-0 w-full max-w-4xl flex-col gap-4'>
            {/* Only the message thread scrolls, so the reply box stays put. */}
            <div ref={scrollRef} className='min-h-0 flex-1 overflow-y-auto pr-1'>
              <div className='space-y-4'>
                {loading ? (
                  <div className='space-y-2'>
                    <Skeleton className='h-4 w-32' />
                    <Skeleton className='h-16 w-3/4' />
                  </div>
                ) : (
                  messages.map((msg) => (
                    <TicketMessageBubble
                      key={msg.id}
                      message={msg}
                      isOwn={msg.sender_id === auth.user?.id}
                    />
                  ))
                )}
              </div>
            </div>

            {/* Reply box — replying on a closed ticket reopens it */}
            <div className='shrink-0 space-y-3 border-t pt-4'>
              <TicketEditor
                value={replyContent}
                onChange={setReplyContent}
                placeholder={t('Type your reply...')}
                minHeight={100}
              />
              <div className='flex justify-end'>
                <Button
                  onClick={handleReply}
                  disabled={sending || !replyContent.trim()}
                >
                  {sending ? t('Sending...') : t('Send Reply')}
                </Button>
              </div>
            </div>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <AlertDialog open={closeConfirm} onOpenChange={setCloseConfirm}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Close Ticket')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'Are you sure you want to close this ticket? You can reopen it by sending a new reply.'
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={closing}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction onClick={handleClose} disabled={closing}>
              {closing ? t('Closing...') : t('Close Ticket')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
