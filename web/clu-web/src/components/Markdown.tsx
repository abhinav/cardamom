import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

// Markdown renders user prose (issue descriptions, comment bodies).
// Uses react-markdown + remark-gfm so we get tables, task list
// checkboxes, autolinks, strikethrough out of the box.
//
// Styling comes from the @tailwindcss/typography plugin via the
// `prose` classes; `prose-invert` flips for our dark default.
// Override `prose`'s `max-w-65ch` cap with `max-w-none` so it fills
// the surrounding column.
export function Markdown({
  source,
  className,
}: {
  source: string
  className?: string
}) {
  return (
    <div
      className={
        'prose prose-sm prose-invert max-w-none ' +
        'prose-p:my-2 prose-headings:mt-4 prose-headings:mb-2 ' +
        'prose-pre:my-2 prose-pre:bg-muted prose-pre:border ' +
        'prose-code:bg-muted prose-code:px-1 prose-code:py-0.5 prose-code:rounded prose-code:font-mono prose-code:text-[0.85em] prose-code:before:content-none prose-code:after:content-none ' +
        'prose-a:text-foreground prose-a:underline prose-a:decoration-muted-foreground ' +
        'prose-hr:border-border ' +
        'prose-blockquote:border-l-border prose-blockquote:text-muted-foreground ' +
        (className ?? '')
      }
    >
      <ReactMarkdown remarkPlugins={[remarkGfm]}>{source}</ReactMarkdown>
    </div>
  )
}
