import { Switch } from 'antd'
import { useState } from 'react'

export function CommandSwitch(props: {
  deviceName: string
  onSend: (cmd: string) => void
}) {
  const [on, setOn] = useState(false)
  return (
    <span>
      LED{' '}
      <Switch
        checked={on}
        onChange={(v) => {
          setOn(v)
          props.onSend(v ? 'LED:ON' : 'LED:OFF')
        }}
      />
    </span>
  )
}
