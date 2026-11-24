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
import { ImagePlus, Loader2 } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { fetchTicketImageBlob, uploadTicketImage } from '@/features/tickets/api'
import { cn } from '@/lib/utils'

const MAX_IMAGE_SIZE = 15 * 1024 * 1024 // 15 MB
const ALLOWED_IMAGE_TYPES = new Set([
  'image/jpeg',
  'image/png',
  'image/gif',
  'image/webp',
])

interface TicketEditorProps {
  value: string
  onChange: (html: string) => void
  placeholder?: string
  disabled?: boolean
  minHeight?: number
  /** Cap the writing area so long content never squeezes the thread above. */
  maxHeight?: number
}

/**
 * Lightweight rich-text editor for ticket content.
 * Accepts plain text + inline images only. File attachments are rejected —
 * pasted images are uploaded immediately and inserted as <img> tags.
 */
export function TicketEditor({
  value,
  onChange,
  placeholder,
  disabled,
  minHeight = 120,
  maxHeight = 240,
}: TicketEditorProps) {
  const { t } = useTranslation()
  const editorRef = useRef<HTMLDivElement>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [uploading, setUploading] = useState(false)

  // Sync external value into the contentEditable only when it diverges —
  // resetting innerHTML on every keystroke would destroy the caret position.
  // Track the last HTML we pushed out so a parent echo of our own onChange
  // is not mistaken for an external update.
  const lastEmittedRef = useRef<string>(value)
  useEffect(() => {
    const editor = editorRef.current
    if (editor && value !== lastEmittedRef.current) {
      editor.innerHTML = value
      lastEmittedRef.current = value
    }
  }, [value])

  // Map blob URL → original server path so emitChange writes the persistent
  // URL into the HTML that gets stored, not the temporary blob.
  const blobToServerRef = useRef<Record<string, string>>({})

  const emitChange = () => {
    let html = editorRef.current?.innerHTML ?? ''
    // Persist the server URL, not the blob URL that only lives for this page.
    for (const [blob, server] of Object.entries(blobToServerRef.current)) {
      html = html.split(`src="${blob}"`).join(`src="${server}"`)
    }
    lastEmittedRef.current = html
    onChange(html)
  }

  const insertImage = (blobUrl: string, serverUrl: string) => {
    blobToServerRef.current[blobUrl] = serverUrl
    const editor = editorRef.current
    if (!editor) return
    editor.focus()
    const img = document.createElement('img')
    img.src = blobUrl
    img.className = 'ticket-editor-image'
    img.style.maxWidth = '100%'
    img.style.borderRadius = '6px'
    img.style.display = 'block'
    img.style.margin = '4px 0'
    const selection = window.getSelection()
    if (selection && selection.rangeCount > 0 && editor.contains(selection.anchorNode)) {
      const range = selection.getRangeAt(0)
      range.deleteContents()
      range.insertNode(img)
      range.setStartAfter(img)
      range.collapse(true)
      selection.removeAllRanges()
      selection.addRange(range)
    } else {
      editor.appendChild(img)
    }
    emitChange()
  }

  const handleFiles = async (files: FileList | File[]) => {
    const fileArray = [...files]
    for (const file of fileArray) {
      if (!ALLOWED_IMAGE_TYPES.has(file.type)) {
        toast.error(t('Only JPEG, PNG, GIF and WebP images are allowed'))
        continue
      }
      if (file.size > MAX_IMAGE_SIZE) {
        toast.error(t('Image exceeds 15 MB limit'))
        continue
      }
      setUploading(true)
      try {
        const res = await uploadTicketImage(file)
        if (res.success && res.data?.url) {
          // Fetch through the auth interceptor so the editor can render the
          // image without a bare <img> request (which carries no credential).
          try {
            const blobUrl = await fetchTicketImageBlob(res.data.url)
            insertImage(blobUrl, res.data.url)
          } catch {
            toast.error(t('Image upload failed'))
          }
        } else {
          toast.error(res.message || t('Image upload failed'))
        }
      } catch {
        toast.error(t('Image upload failed'))
      } finally {
        setUploading(false)
      }
    }
  }

  const handlePaste = (e: React.ClipboardEvent<HTMLDivElement>) => {
    const items = e.clipboardData?.items
    if (!items) return

    const imageFiles: File[] = []
    let hasNonImageFile = false
    for (const item of items) {
      if (item.kind === 'file') {
        const file = item.getAsFile()
        if (file && ALLOWED_IMAGE_TYPES.has(file.type)) {
          imageFiles.push(file)
        } else {
          hasNonImageFile = true
        }
      }
    }

    if (hasNonImageFile) {
      e.preventDefault()
      toast.warning(t('Attachments are not supported, only images'))
      return
    }

    if (imageFiles.length > 0) {
      e.preventDefault()
      void handleFiles(imageFiles)
      return
    }

    // Plain text paste: strip formatting, keep text only
    e.preventDefault()
    const text = e.clipboardData.getData('text/plain')
    document.execCommand('insertText', false, text)
  }

  const handleDrop = (e: React.DragEvent<HTMLDivElement>) => {
    e.preventDefault()
    const files = [...(e.dataTransfer?.files ?? [])]
    if (files.length === 0) return
    const nonImage = files.some((f) => !ALLOWED_IMAGE_TYPES.has(f.type))
    if (nonImage) {
      toast.warning(t('Attachments are not supported, only images'))
      return
    }
    void handleFiles(files)
  }

  const handleDragOver = (e: React.DragEvent<HTMLDivElement>) => {
    e.preventDefault()
  }

  const handleKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    // Enter = new line, Shift+Enter = new line (same); Ctrl/Cmd+Enter = submit
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
      e.preventDefault()
      const form = editorRef.current?.closest('form')
      if (form) {
        form.requestSubmit()
      }
    }
  }

  return (
    <div className='space-y-2'>
      <div
        ref={editorRef}
        contentEditable={!disabled}
        role='textbox'
        aria-multiline='true'
        aria-label={placeholder}
        data-placeholder={placeholder}
        onInput={emitChange}
        onPaste={handlePaste}
        onDrop={handleDrop}
        onDragOver={handleDragOver}
        onKeyDown={handleKeyDown}
        style={{ minHeight, maxHeight, overflowY: 'auto' }}
        className={cn(
          'border-input focus-visible:border-ring focus-visible:ring-ring/50 w-full rounded-md border bg-transparent px-3 py-2 text-sm shadow-xs transition-[color,box-shadow] outline-none focus-visible:ring-[3px]',
          'empty:before:text-muted-foreground empty:before:content-[attr(data-placeholder)]',
          '[&_img]:max-w-full [&_img]:rounded-md [&_img]:my-1',
          disabled && 'cursor-not-allowed opacity-50'
        )}
      />

      <div className='flex items-center justify-between'>
        <div className='text-muted-foreground text-xs'>
          {t('Paste images directly, attachments not supported')}
        </div>
        <Button
          type='button'
          variant='ghost'
          size='sm'
          disabled={disabled || uploading}
          onClick={() => fileInputRef.current?.click()}
        >
          {uploading ? (
            <Loader2 className='mr-1 h-4 w-4 animate-spin' />
          ) : (
            <ImagePlus className='mr-1 h-4 w-4' />
          )}
          {t('Insert image')}
        </Button>
      </div>

      <input
        ref={fileInputRef}
        type='file'
        accept='image/jpeg,image/png,image/gif,image/webp'
        multiple
        className='hidden'
        onChange={(e) => {
          if (e.target.files?.length) {
            void handleFiles(e.target.files)
          }
          e.target.value = ''
        }}
      />
    </div>
  )
}
