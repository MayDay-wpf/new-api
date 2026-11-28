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
import { Link } from '@tanstack/react-router'
import { ChevronLeft, ChevronRight, Plus } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

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
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { SectionPageLayout } from '@/components/layout'
import { Dialog } from '@/components/dialog'

import { createTicket, getUserTickets } from './api'
import { TicketEditor } from './components/ticket-editor'
import { TicketReadBadge } from './components/ticket-read-badge'
import type { Ticket } from './types'

function TicketStatusBadge({ status }: { status: string }) {
  const { t } = useTranslation()
  return (
    <Badge variant={status === 'open' ? 'default' : 'secondary'}>
      {status === 'open' ? t('Open') : t('Closed')}
    </Badge>
  )
}

export function Tickets() {
  const { t } = useTranslation()
  const [tickets, setTickets] = useState<Ticket[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize] = useState(20)
  const [loading, setLoading] = useState(true)
  const [createOpen, setCreateOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [newTitle, setNewTitle] = useState('')
  const [newContent, setNewContent] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await getUserTickets(page, pageSize)
      if (res.success && res.data) {
        setTickets(res.data.items ?? [])
        setTotal(res.data.total)
      }
    } finally {
      setLoading(false)
    }
  }, [page, pageSize])

  useEffect(() => {
    void load()
  }, [load])

  const handleCreate = async () => {
    const title = newTitle.trim()
    const content = newContent.trim()
    if (!title) {
      toast.error(t('Title is required'))
      return
    }
    if (!content) {
      toast.error(t('Content is required'))
      return
    }
    setCreating(true)
    try {
      const res = await createTicket({ title, content })
      if (res.success) {
        toast.success(t('Ticket created'))
        setCreateOpen(false)
        setNewTitle('')
        setNewContent('')
        void load()
      } else {
        toast.error(res.message || t('Failed to create ticket'))
      }
    } finally {
      setCreating(false)
    }
  }

  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  let ticketList: React.ReactNode = null
  if (loading) {
    ticketList = ['a', 'b', 'c'].map((k) => (
      <Card key={k}>
        <CardHeader>
          <Skeleton className='h-5 w-48' />
          <Skeleton className='h-4 w-32' />
        </CardHeader>
      </Card>
    ))
  } else if (tickets.length === 0) {
    ticketList = (
      <Card>
        <CardContent className='text-muted-foreground flex min-h-40 items-center justify-center py-10 text-sm'>
          {t('No tickets yet')}
        </CardContent>
      </Card>
    )
  }

  return (
    <>
      <SectionPageLayout fixedContent>
        <SectionPageLayout.Title>{t('Support Tickets')}</SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <Button size='sm' onClick={() => setCreateOpen(true)}>
            <Plus className='mr-1 h-4 w-4' />
            {t('New Ticket')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='mx-auto flex h-full w-full max-w-4xl flex-col gap-3 overflow-y-auto pr-1'>
            {ticketList}
            {!loading &&
              tickets.map((ticket) => (
                <Link
                  key={ticket.id}
                  to='/tickets/$ticketId'
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
                          <TicketStatusBadge status={ticket.status} />
                        </div>
                      </div>
                      <CardDescription>
                        #{ticket.id} · {ticket.message_count} {t('messages')}{' '}
                        · {new Date(ticket.updated_at * 1000).toLocaleString()}
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

      <Dialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        title={t('New Ticket')}
        description={t('Describe your issue. Paste images directly if needed.')}
        footer={
          <>
            <Button
              variant='outline'
              onClick={() => setCreateOpen(false)}
              disabled={creating}
            >
              {t('Cancel')}
            </Button>
            <Button onClick={handleCreate} disabled={creating}>
              {creating ? t('Creating...') : t('Submit')}
            </Button>
          </>
        }
      >
        <div className='space-y-4'>
          <div className='space-y-2'>
            <Label htmlFor='ticket-title'>{t('Title')}</Label>
            <Input
              id='ticket-title'
              value={newTitle}
              onChange={(e) => setNewTitle(e.target.value)}
              placeholder={t('Brief summary of your issue')}
              maxLength={200}
            />
          </div>
          <div className='space-y-2'>
            <Label>{t('Description')}</Label>
            <TicketEditor
              value={newContent}
              onChange={setNewContent}
              placeholder={t('Describe your problem in detail...')}
            />
          </div>
        </div>
      </Dialog>
    </>
  )
}
