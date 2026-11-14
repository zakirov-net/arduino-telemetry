import type { AggregatedReading, Bucket, Device } from './types'

const base = '/api'

export async function fetchDevices(): Promise<Device[]> {
  const r = await fetch(`${base}/devices`)
  if (!r.ok) throw new Error(`devices: ${r.status}`)
  const json = await r.json()
  return Array.isArray(json) ? json : []
}

export async function fetchReadings(params: {
  deviceId: number
  from: string
  to: string
  bucket: Bucket
}): Promise<AggregatedReading[]> {
  const q = new URLSearchParams({
    device_id: String(params.deviceId),
    from: params.from,
    to: params.to,
    bucket: params.bucket,
  })
  const r = await fetch(`${base}/readings?${q}`)
  if (!r.ok) throw new Error(`readings: ${r.status}`)
  const json = await r.json()
  return Array.isArray(json) ? json : []
}

export async function sendCommand(deviceName: string, cmd: string): Promise<void> {
  const r = await fetch(`${base}/command`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ device_name: deviceName, command: cmd }),
  })
  if (!r.ok) throw new Error(`command: ${r.status}`)
}
