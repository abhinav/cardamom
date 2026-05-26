import { useLiveStatus } from '../lib/use-change-stream'
import { Tooltip, TooltipContent, TooltipTrigger } from './ui/tooltip'

// LiveIndicator renders the SSE connection state as a tiny coloured
// dot with a tooltip. Replaces the old "Refresh" button in page
// headers — refresh is meaningless now that the server pushes.
//
// green   = live (events flowing)
// amber   = connecting (initial or reconnecting)
// red     = offline (EventSource error; browser is retrying)
export default function LiveIndicator() {
  const status = useLiveStatus()
  const { dotClass, label, hint } =
    status === 'live'
      ? {
          dotClass: 'bg-emerald-500 shadow-[0_0_8px_oklch(0.72_0.16_155/0.6)]',
          label: 'Live',
          hint: 'Connected to the change stream — updates push in real time.',
        }
      : status === 'connecting'
        ? {
            dotClass: 'bg-amber-500 animate-pulse',
            label: 'Connecting',
            hint: 'Waiting for the change stream to wire through.',
          }
        : {
            dotClass: 'bg-rose-500',
            label: 'Offline',
            hint: 'Change stream is offline; retrying. Refresh manually if needed.',
          }
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="text-muted-foreground hover:text-foreground inline-flex h-8 items-center gap-1.5 rounded-md px-2 text-xs font-medium">
          <span className={`size-1.5 rounded-full ${dotClass}`} />
          {label}
        </span>
      </TooltipTrigger>
      <TooltipContent side="bottom">{hint}</TooltipContent>
    </Tooltip>
  )
}
