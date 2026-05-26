import {
  Circle,
  CircleDashed,
  CircleDot,
  CircleSlash,
  CircleX,
} from 'lucide-react'
import { Badge } from './ui/badge'
import type { Status } from '../lib/issue-display'

interface Props {
  status: Status
  blocked?: boolean
  className?: string
}

// StatusBadge — single source of truth for how an issue's effective
// state surfaces in the UI. `open + blocked` becomes its own "blocked"
// pill (mirrors the CLI's displayStatus). Colour-coded via the Badge
// component's status variants.
export function StatusBadge({ status, blocked, className }: Props) {
  if (status === 'open' && blocked) {
    return (
      <Badge variant="danger" className={className}>
        <CircleSlash />
        blocked
      </Badge>
    )
  }
  switch (status) {
    case 'open':
      return (
        <Badge variant="success" className={className}>
          <Circle />
          open
        </Badge>
      )
    case 'in_progress':
      return (
        <Badge variant="warning" className={className}>
          <CircleDot />
          in progress
        </Badge>
      )
    case 'closed':
      return (
        <Badge variant="muted" className={className}>
          <CircleDashed />
          closed
        </Badge>
      )
    case 'cancelled':
      return (
        <Badge variant="muted" className={className}>
          <CircleX />
          cancelled
        </Badge>
      )
  }
}

// PriorityBadge — small mono p0/p1/p2 chip; colour maps urgency.
export function PriorityBadge({
  priority,
  className,
}: {
  priority: number
  className?: string
}) {
  const variant =
    priority <= 0 ? 'danger' : priority === 1 ? 'warning' : priority === 2 ? 'info' : 'muted'
  return (
    <Badge variant={variant} className={`font-mono ${className ?? ''}`}>
      p{priority}
    </Badge>
  )
}
