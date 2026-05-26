import { useEffect, useMemo } from 'react'
import {
  Background,
  BackgroundVariant,
  Handle,
  Position,
  ReactFlow,
  ReactFlowProvider,
  useReactFlow,
  type Edge,
  type Node,
  type NodeProps,
} from '@xyflow/react'
import dagre from 'dagre'
import { ShieldCheck, UserRound } from 'lucide-react'
import type { Plan, PlanStep } from '../lib/api'
import { PriorityBadge } from './StatusBadge'

import '@xyflow/react/dist/style.css'

// Node geometry. Width is fixed so the dagre layout is deterministic;
// height comes from the rendered content but we approximate for
// layout so edges don't overlap nodes on first paint.
const NODE_W = 220
const NODE_H = 84

// ---- custom node types ----

interface StepNodeData extends Record<string, unknown> {
  step: PlanStep
}

// StepNode — one box per workflow step. Renders type-flavoured
// styling: checkpoints get the amber shield treatment, tasks get
// the neutral card look. Click target reserved for the future
// run-progress view; in preview mode it's purely informational.
function StepNode({ data }: NodeProps<Node<StepNodeData>>) {
  const s = data.step
  const isCheckpoint = s.type === 'checkpoint'
  return (
    <div
      className={
        'group relative w-[220px] rounded-md border bg-card text-card-foreground shadow-sm transition-colors ' +
        (isCheckpoint
          ? 'border-amber-500/40 bg-amber-500/[0.04]'
          : 'border-border')
      }
    >
      <Handle
        type="target"
        position={Position.Left}
        className="!h-1.5 !w-1.5 !border-0 !bg-muted-foreground/60"
      />
      <div className="flex items-center gap-1.5 border-b px-2.5 py-1.5 text-[10px] uppercase tracking-wider">
        {isCheckpoint ? (
          <ShieldCheck className="size-3 text-amber-400" />
        ) : (
          <span className="size-1.5 rounded-full bg-emerald-500/70" />
        )}
        <span className="text-muted-foreground font-mono">{s.id}</span>
        <PriorityBadge priority={s.priority} className="ml-auto !px-1 !py-0" />
      </div>
      <div className="px-2.5 py-2 text-[12px] leading-snug font-medium">
        {s.title}
      </div>
      {(s.agent || s.is_leaf) && (
        <div className="text-muted-foreground flex items-center gap-1.5 border-t px-2.5 py-1 text-[10px]">
          {s.agent && (
            <span className="inline-flex items-center gap-0.5">
              <UserRound className="size-2.5" />@{s.agent}
            </span>
          )}
          {s.is_leaf && (
            <span className="ml-auto" title="parent depends on this">
              leaf
            </span>
          )}
        </div>
      )}
      <Handle
        type="source"
        position={Position.Right}
        className="!h-1.5 !w-1.5 !border-0 !bg-muted-foreground/60"
      />
    </div>
  )
}

const nodeTypes = { step: StepNode }

// ParentNode — anchor at the top of the graph showing the parent
// issue title. Renders distinct from steps (rounded-full, primary
// surface) so the user sees the run hierarchy at a glance.
function ParentNode({ data }: NodeProps<Node<{ title: string }>>) {
  return (
    <div className="bg-primary text-primary-foreground rounded-full px-4 py-1.5 text-xs font-semibold shadow-md">
      {data.title}
      <Handle
        type="source"
        position={Position.Right}
        className="!h-1.5 !w-1.5 !border-0 !bg-primary-foreground/60"
      />
    </div>
  )
}

nodeTypes.parent = ParentNode as typeof StepNode

// ---- layout ----

// layoutPlan runs dagre over the step graph and returns nodes + edges
// positioned for ReactFlow. Edges follow the `needs:` chain (parent
// step → dependent step). The synthetic parent node sits at the top,
// connecting down to every step the user didn't tag as needed-by-
// something (i.e. depth-0 starting points).
function layoutPlan(plan: Plan): { nodes: Node[]; edges: Edge[] } {
  const g = new dagre.graphlib.Graph()
  g.setGraph({ rankdir: 'LR', nodesep: 24, ranksep: 70, marginx: 16, marginy: 16 })
  g.setDefaultEdgeLabel(() => ({}))

  const parentID = '__parent__'
  g.setNode(parentID, { width: 280, height: 32 })
  for (const s of plan.steps) {
    g.setNode(s.id, { width: NODE_W, height: NODE_H })
  }

  // Edges: needs chain + synthetic parent → roots (steps with no needs).
  const edges: Edge[] = []
  for (const s of plan.steps) {
    if (!s.needs || s.needs.length === 0) {
      g.setEdge(parentID, s.id)
      edges.push({
        id: `${parentID}->${s.id}`,
        source: parentID,
        target: s.id,
        type: 'smoothstep',
        style: { stroke: 'oklch(0.55 0.012 286)', strokeDasharray: '4 4' },
      })
      continue
    }
    for (const n of s.needs) {
      g.setEdge(n, s.id)
      edges.push({
        id: `${n}->${s.id}`,
        source: n,
        target: s.id,
        type: 'smoothstep',
        style: { stroke: 'oklch(0.6 0.014 286)' },
      })
    }
  }

  dagre.layout(g)

  const nodes: Node[] = []
  const p = g.node(parentID)
  nodes.push({
    id: parentID,
    type: 'parent',
    // dagre returns centre coords; ReactFlow expects top-left
    position: { x: p.x - 140, y: p.y - 16 },
    data: { title: plan.title },
    draggable: false,
    selectable: false,
  })
  for (const s of plan.steps) {
    const pos = g.node(s.id)
    nodes.push({
      id: s.id,
      type: 'step',
      position: { x: pos.x - NODE_W / 2, y: pos.y - NODE_H / 2 },
      data: { step: s },
      draggable: false,
    })
  }

  return { nodes, edges }
}

// ---- public component ----

// Inner component sits below a ReactFlowProvider so it can call
// useReactFlow() and re-fit on plan changes — ReactFlow's `fitView`
// prop only runs once at mount, which causes clipping if the graph
// is wider than the container at first render (common in dialogs
// that animate open).
function GraphInner({ plan }: { plan: Plan }) {
  const { nodes, edges } = useMemo(() => layoutPlan(plan), [plan])
  const rf = useReactFlow()
  useEffect(() => {
    // Defer one frame so the container has measured its final size
    // before fitView reads it. Padding 0.15 leaves a comfortable
    // margin without zooming so far in that small graphs feel sparse.
    const id = requestAnimationFrame(() => {
      rf.fitView({ padding: 0.15, duration: 200 })
    })
    return () => cancelAnimationFrame(id)
  }, [nodes, edges, rf])
  return (
    <ReactFlow
      nodes={nodes}
      edges={edges}
      nodeTypes={nodeTypes}
      fitView
      fitViewOptions={{ padding: 0.15 }}
      proOptions={{ hideAttribution: true }}
      nodesDraggable={false}
      nodesConnectable={false}
      zoomOnDoubleClick={false}
      panOnScroll={false}
      minZoom={0.2}
      defaultEdgeOptions={{ animated: false }}
    >
      <Background
        variant={BackgroundVariant.Dots}
        gap={14}
        size={1}
        color="oklch(0.4 0.005 285.8 / 0.5)"
      />
    </ReactFlow>
  )
}

export function WorkflowGraph({
  plan,
  className,
}: {
  plan: Plan
  className?: string
}) {
  return (
    <div className={'rounded-md border bg-muted/20 ' + (className ?? '')}>
      <ReactFlowProvider>
        <GraphInner plan={plan} />
      </ReactFlowProvider>
    </div>
  )
}
