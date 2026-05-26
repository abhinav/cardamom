import { Toaster as Sonner, type ToasterProps } from 'sonner'

// Sonner wrapper that picks up shadcn's CSS variables. The
// invertable colours come from --popover / --popover-foreground so
// toasts blend with the rest of the popover surfaces.
function Toaster(props: ToasterProps) {
  return (
    <Sonner
      theme="system"
      className="toaster group"
      position="bottom-right"
      style={
        {
          '--normal-bg': 'var(--popover)',
          '--normal-text': 'var(--popover-foreground)',
          '--normal-border': 'var(--border)',
        } as React.CSSProperties
      }
      {...props}
    />
  )
}

export { Toaster }
