import React, {
  useCallback,
  useEffect,
  useRef,
  useState,
} from 'react'
import { Tooltip, theme } from 'antd'
import { CommentOutlined } from '@ant-design/icons'

/** 可拖动的「智能助手」悬浮入口，默认右下角，位置记入 localStorage；侧栏菜单仍为固定导航链接。 */
const STORAGE_KEY = 'opinion_assistant_launcher_pos_v1'
const BTN = 56
const MARGIN = 16
const DRAG_THRESHOLD_PX = 8

type Pos = { left: number; top: number }

function clamp(n: number, lo: number, hi: number) {
  return Math.min(hi, Math.max(lo, n))
}

function defaultPos(): Pos {
  if (typeof window === 'undefined') {
    return { left: 100, top: 100 }
  }
  return {
    left: window.innerWidth - MARGIN - BTN,
    top: window.innerHeight - MARGIN - BTN,
  }
}

function loadPos(): Pos {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return defaultPos()
    const p = JSON.parse(raw) as unknown
    if (
      !p ||
      typeof p !== 'object' ||
      typeof (p as Pos).left !== 'number' ||
      typeof (p as Pos).top !== 'number'
    ) {
      return defaultPos()
    }
    return clampPos((p as Pos).left, (p as Pos).top)
  } catch {
    return defaultPos()
  }
}

function savePos(left: number, top: number) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ left, top }))
  } catch {
    /* quota / private mode */
  }
}

function clampPos(left: number, top: number): Pos {
  if (typeof window === 'undefined') {
    return { left, top }
  }
  const maxL = Math.max(MARGIN, window.innerWidth - BTN - MARGIN)
  const maxT = Math.max(MARGIN, window.innerHeight - BTN - MARGIN)
  return {
    left: clamp(left, MARGIN, maxL),
    top: clamp(top, MARGIN, maxT),
  }
}

type DragState = {
  pointerId: number
  startX: number
  startY: number
  originL: number
  originT: number
  moved: boolean
}

type Props = {
  onOpen: () => void
}

const DraggableAssistantLauncher: React.FC<Props> = ({ onOpen }) => {
  const { token } = theme.useToken()
  const [pos, setPos] = useState<Pos>(() => loadPos())
  const [grabbing, setGrabbing] = useState(false)
  const dragRef = useRef<DragState | null>(null)

  useEffect(() => {
    const onResize = () => {
      setPos((p) => clampPos(p.left, p.top))
    }
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [])

  const endDrag = useCallback(
    (pointerId: number, el: HTMLButtonElement | null) => {
      const d = dragRef.current
      if (!d || d.pointerId !== pointerId) return
      dragRef.current = null
      setGrabbing(false)
      if (el) {
        try {
          el.releasePointerCapture(pointerId)
        } catch {
          /* already released */
        }
      }
      setPos((p) => {
        const c = clampPos(p.left, p.top)
        savePos(c.left, c.top)
        return c
      })
      if (!d.moved) {
        onOpen()
      }
    },
    [onOpen]
  )

  const shadow =
    typeof token.boxShadowSecondary === 'string'
      ? token.boxShadowSecondary
      : '0 6px 16px rgba(0, 0, 0, 0.12)'

  return (
    <Tooltip title="按住拖动可移动 · 单击打开智能助手" placement="left">
      <button
        type="button"
        aria-label="打开智能助手"
        style={{
          position: 'fixed',
          zIndex: 1000,
          left: pos.left,
          top: pos.top,
          width: BTN,
          height: BTN,
          border: 'none',
          borderRadius: '50%',
          cursor: grabbing ? 'grabbing' : 'grab',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: '#fff',
          background: token.colorPrimary,
          boxShadow: shadow,
          touchAction: 'none',
          userSelect: 'none',
        }}
        onPointerDown={(e) => {
          if (e.button !== 0) return
          e.currentTarget.setPointerCapture(e.pointerId)
          dragRef.current = {
            pointerId: e.pointerId,
            startX: e.clientX,
            startY: e.clientY,
            originL: pos.left,
            originT: pos.top,
            moved: false,
          }
        }}
        onPointerMove={(e) => {
          const d = dragRef.current
          if (!d || e.pointerId !== d.pointerId) return
          const dx = e.clientX - d.startX
          const dy = e.clientY - d.startY
          if (dx * dx + dy * dy > DRAG_THRESHOLD_PX * DRAG_THRESHOLD_PX) {
            if (!d.moved) setGrabbing(true)
            d.moved = true
          }
          setPos(clampPos(d.originL + dx, d.originT + dy))
        }}
        onPointerUp={(e) => endDrag(e.pointerId, e.currentTarget)}
        onPointerCancel={(e) => endDrag(e.pointerId, e.currentTarget)}
        onLostPointerCapture={(e) => {
          endDrag(e.pointerId, e.currentTarget)
        }}
      >
        <CommentOutlined style={{ fontSize: 22 }} />
      </button>
    </Tooltip>
  )
}

export default DraggableAssistantLauncher
