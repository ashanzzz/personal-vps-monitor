import { Resizable } from 'react-resizable'
import type { ResizeCallbackData } from 'react-resizable'
import 'react-resizable/css/styles.css'

interface ResizableTitleProps extends React.HTMLAttributes<HTMLTableCellElement> {
  onResize?: (e: React.SyntheticEvent, data: ResizeCallbackData) => void
  width?: number
}

/**
 * antd Table header cell wrapped with a drag handle so column widths can be
 * resized. Falls back to a plain <th> for columns that declare no width.
 */
export function ResizableTitle(props: ResizableTitleProps) {
  const { onResize, width, ...restProps } = props

  if (!width) {
    return <th {...restProps} />
  }

  return (
    <Resizable
      width={width}
      height={0}
      handle={
        <span
          className="react-resizable-handle"
          // Stop the click reaching the column header's sorter toggle.
          onClick={e => e.stopPropagation()}
        />
      }
      onResize={onResize}
      minConstraints={[60, 0]}
    >
      <th {...restProps} />
    </Resizable>
  )
}
