import type { Book } from '@/types'

// Formats that have an in-browser reader (image CBZ reader, PDF viewer, EPUB).
const READABLE_FORMATS = ['cbz', 'pdf', 'epub']

// isReadable reports whether a book can be opened in one of the in-browser
// readers, so a cover thumbnail can link straight to /books/:id/read.
export function isReadable(book: Pick<Book, 'formats'>): boolean {
  return book.formats.some((f) => READABLE_FORMATS.includes(f.format.toLowerCase()))
}
