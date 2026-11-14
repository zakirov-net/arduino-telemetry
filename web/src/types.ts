export interface Device {
  id: number
  name: string
  location: string | null
}

export interface AggregatedReading {
  recorded_at: string
  temperature: number | null
}

export type Bucket = '1m' | '5m' | '1h'
