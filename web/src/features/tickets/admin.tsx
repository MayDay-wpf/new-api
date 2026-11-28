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
import {
  ArrowLeft,
  ChevronLeft,
  ChevronRight,
  Lock,
  Search,
} from 'lucide-react'
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
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { SectionPageLayout } from '@/components/layout'
import { useAuthStore } from '@/stores/auth-store'

import {
  adminCloseTicket,
  adminGetTicketDetail,
  adminGetTickets,
  adminReplyTicket,
} from './api'
import { useInvalidateTicketUnread } from './hooks/use-ticket-badge'
import { useStickToBottom } from './hooks/use-stick-to-bottom'
import { TicketEditor } from './components/ticket-editor'
import { TicketMessageBubble } from './components/ticket-message-bubble'
import { TicketReadBadge } from './components/ticket-read-badge'
import type { Ticket, TicketMessage } from './types'

// ---------------------------------------------------------------------------
// Admin ticket list
// ---------------------------------------------------------------------------

export function AdminTickets() {
  const { t } = useTranslation()
  const [tickets, setTickets] = useState<Ticket[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize] = useState(20)
  const [status, setStatus] = useState('')
  const [keyword, setKeyword] = useState('')
  const [keywordInput, setKeywordInput] = useState('')
  const [loading, setLoading] = useState(true)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await adminGetTickets({
        page,
        page_size: pageSize,
        status: status || undefined,
        keyword: keyword || undefined,
      })
      if (res.success && res.data) {
        setTickets(res.data.items ?? [])
        setTotal(res.data.total)
      }
    } finally {
      setLoading(false)
    }
  }, [page, pageSize, status, keyword])

  useEffect(() => {
    void load()
  }, [load])

  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  let listContent: React.ReactNode = null
  if (loading) {
    listContent = ['a', 'b', 'c'].map((k) => (
      <Card key={k}>
        <CardHeader>
          <Skeleton className='h-5 w-64' />
          <Skeleton className='h-4 w-48' />
        </CardHeader>
      </Card>
    ))
  } else if (tickets.length === 0) {
    listContent = (
      <Card>
        <CardContent className='text-muted-foreground flex min-h-40 items-center justify-center py-10 text-sm'>
          {t('No tickets found')}
        </CardContent>
      </Card>
    )
  }

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>
        {t('Ticket Management')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <div className='relative w-48 sm:w-64'>
          <Search className='text-muted-foreground absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2' />
          <Input
            className='h-8 pl-9'
            placeholder={t('Search tickets...')}
            value={keywordInput}
            onChange={(e) => setKeywordInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                setKeyword(keywordInput)
                setPage(1)
              }
            }}
          />
        </div>
        <Select
          value={status || 'all'}
          onValueChange={(v) => {
            setStatus(v === null || v === 'all' ? '' : v)
            setPage(1)
          }}
        >
          <SelectTrigger className='h-8 w-28'>
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            <SelectGroup>
              <SelectItem value='all'>{t('All')}</SelectItem>
              <SelectItem value='open'>{t('Open')}</SelectItem>
              <SelectItem value='closed'>{t('Closed')}</SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='mx-auto flex h-full w-full max-w-5xl flex-col gap-3 overflow-y-auto pr-1'>
          {listContent}
          {!loading &&
            tickets.map((ticket) => (
              <Link
                key={ticket.id}
                to='/tickets/admin/$ticketId'
                params={{ ticketId: String(ticket.id) }}
              >
                <Card className='hover:bg-muted/50 cursor-pointer transition-colors'>
                  <CardHeader className='pb-3'>
                    <div className='flex items-start justify-between gap-2'>
                      <CardTitle className='min-w-0 truncate text-base'>
                        {ticket.title}
                      </CardTitle>
                      <div className='flex shrink-0 items-center gap-2'>
                        <TicketReadBadge unread={ticket.unread} />
                        <Badge
                          variant={
                            ticket.status === 'open' ? 'default' : 'secondary'
                          }
                        >
                          {ticket.status === 'open'
                            ? t('Open')
                            : t('Closed')}
                        </Badge>
                      </div>
                    </div>
                    <CardDescription className='mt-1'>
                      #{ticket.id} ·{' '}
                      {ticket.username ||
                        ticket.display_name ||
                        `#${ticket.user_id}`}{' '}
                      · {ticket.message_count} {t('messages')} ·{' '}
                      {new Date(
                        ticket.updated_at * 1000
                      ).toLocaleString()}
                    </CardDescription>
                  </CardHeader>
                </Card>
              </Link>
            ))}

          {total > pageSize && (
            <div className='flex items-center justify-between border-t pt-3'>
              <div className='text-muted-foreground text-sm'>
                {t('Showing')} {(page - 1) * pageSize + 1}–
                {Math.min(page * pageSize, total)} {t('of')} {total}
              </div>
              <div className='flex items-center gap-2'>
                <Button
                  variant='outline'
                  size='sm'
                  disabled={page <= 1}
                  onClick={() => setPage((p) => p - 1)}
                >
                  <ChevronLeft className='h-4 w-4' />
                </Button>
                <span className='text-sm'>
                  {page} / {totalPages}
                </span>
                <Button
                  variant='outline'
                  size='sm'
                  disabled={page >= totalPages}
                  onClick={() => setPage((p) => p + 1)}
                >
                  <ChevronRight className='h-4 w-4' />
                </Button>
              </div>
            </div>
          )}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

// ---------------------------------------------------------------------------
// Admin ticket detail (reply + close)
// ---------------------------------------------------------------------------

export function AdminTicketDetail() {
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
      const res = await adminGetTicketDetail(id)
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
      const res = await adminReplyTicket(id, content)
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
      const res = await adminCloseTicket(id)
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
            <Link to='/tickets/admin'>
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
            {/* Meta */}
            {ticket && (
              <div className='text-muted-foreground shrink-0 text-sm'>
                #{ticket.id} ·{' '}
                {ticket.username ||
                  ticket.display_name ||
                  `#${ticket.user_id}`}{' '}
                · {new Date(ticket.created_at * 1000).toLocaleString()}
              </div>
            )}

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

            {/* Admin reply — always available; on a closed ticket it reopens */}
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
                'Are you sure you want to close this ticket? The user can reopen it by replying.'
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
