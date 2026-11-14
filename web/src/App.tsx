import { useEffect, useState } from 'react'
import { ConfigProvider, Layout, Select, Radio, Switch, Card, message } from 'antd'
import { LineChart, Line, XAxis, YAxis, Tooltip, CartesianGrid, ResponsiveContainer } from 'recharts'
import { fetchDevices, fetchReadings, sendCommand } from './api'
import type { AggregatedReading, Bucket, Device } from './types'
import { CommandSwitch } from './components/CommandSwitch'
import './styles.css'

const RANGES = {
  '1h':  { ms: 3600_000,    bucket: '1m' as Bucket },
  '24h': { ms: 86_400_000,  bucket: '5m' as Bucket },
  '7d':  { ms: 604_800_000, bucket: '1h' as Bucket },
} as const

type RangeKey = keyof typeof RANGES

export default function App() {
  const [devices, setDevices] = useState<Device[]>([])
  const [deviceId, setDeviceId] = useState<number | null>(null)
  const [range, setRange] = useState<RangeKey>('1h')
  const [autoRefresh, setAutoRefresh] = useState(true)
  const [data, setData] = useState<AggregatedReading[]>([])
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    fetchDevices()
      .then((ds) => {
        setDevices(ds)
        if (ds.length > 0 && deviceId === null) setDeviceId(ds[0].id)
      })
      .catch((e) => message.error(String(e)))
  }, [])

  useEffect(() => {
    if (deviceId === null) return
    const load = async () => {
      setLoading(true)
      try {
        const to = new Date()
        const from = new Date(to.getTime() - RANGES[range].ms)
        const rows = await fetchReadings({
          deviceId,
          from: from.toISOString(),
          to: to.toISOString(),
          bucket: RANGES[range].bucket,
        })
        setData(rows ?? [])
      } catch (e) {
        message.error(String(e))
      } finally {
        setLoading(false)
      }
    }
    load()
    if (!autoRefresh) return
    const t = setInterval(load, 10_000)
    return () => clearInterval(t)
  }, [deviceId, range, autoRefresh])

  const selectedDevice = devices.find((d) => d.id === deviceId)

  return (
    <ConfigProvider>
      <Layout style={{ minHeight: '100vh', padding: 24, background: '#f5f5f5' }}>
        <Card
          title="Arduino Telemetry"
          extra={
            <span>
              Auto-refresh{' '}
              <Switch checked={autoRefresh} onChange={setAutoRefresh} />
            </span>
          }
        >
          <div className="toolbar">
            <Select
              style={{ width: 200 }}
              value={deviceId ?? undefined}
              onChange={setDeviceId}
              options={devices.map((d) => ({
                value: d.id,
                label: `${d.name}${d.location ? ` (${d.location})` : ''}`,
              }))}
            />
            <Radio.Group
              value={range}
              onChange={(e) => setRange(e.target.value)}
              optionType="button"
              buttonStyle="solid"
              options={[
                { label: '1 час', value: '1h' },
                { label: '24 часа', value: '24h' },
                { label: '7 дней', value: '7d' },
              ]}
            />
            {selectedDevice && (
              <CommandSwitch
                deviceName={selectedDevice.name}
                onSend={(cmd) =>
                  sendCommand(selectedDevice.name, cmd)
                    .then(() => message.success(`Отправлено: ${cmd}`))
                    .catch((e) => message.error(String(e)))
                }
              />
            )}
          </div>

          <div style={{ height: 420, marginTop: 24 }}>
            {loading && data.length === 0 ? (
              <p>Загрузка...</p>
            ) : (
              <ResponsiveContainer width="100%" height="100%">
                <LineChart data={data}>
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis
                    dataKey="recorded_at"
                    tickFormatter={(v) => new Date(v).toLocaleString()}
                  />
                  <YAxis domain={['auto', 'auto']} unit="°C" />
                  <Tooltip
                    labelFormatter={(v) => new Date(v as string).toLocaleString()}
                    formatter={(v) => [`${Number(v).toFixed(2)} °C`, 'Температура']}
                  />
                  <Line
                    type="monotone"
                    dataKey="temperature"
                    stroke="#1677ff"
                    dot={false}
                    isAnimationActive={false}
                  />
                </LineChart>
              </ResponsiveContainer>
            )}
          </div>
        </Card>
      </Layout>
    </ConfigProvider>
  )
}
