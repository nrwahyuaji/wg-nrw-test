// Grafik trafik (F-6.1): per peer atau per interface, pilihan 24 jam / 7 hari / 30 hari. Titik dari
// GET /peers/{id}/traffic atau /interfaces/{id}/traffic; ember kosong sudah diisi 0 oleh server.
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { apiFetch, type TrafficPoint } from '../api/client'
import { chartLabel, formatBytes } from '../lib/format'
import { Button } from './ui/button'
import { Card } from './ui/card'

type Range = '24h' | '7d' | '30d'
const RANGES: [Range, string][] = [['24h', '24 jam'], ['7d', '7 hari'], ['30d', '30 hari']]

/** TrafficChart: `scope` = path sumber, mis. "/peers/3" atau "/interfaces/1". Dimuat ulang tiap menit (= sampler). */
export function TrafficChart({ scope, title = 'Trafik' }: { scope: string; title?: string }) {
  const [range, setRange] = useState<Range>('24h')
  const traffic = useQuery({
    queryKey: ['traffic', scope, range],
    queryFn: () => apiFetch<TrafficPoint[]>(`${scope}/traffic?range=${range}`),
    refetchInterval: 60_000, refetchIntervalInBackground: false,
  })
  const points = traffic.data ?? []
  const totalRx = points.reduce((sum, point) => sum + point.rx, 0)
  const totalTx = points.reduce((sum, point) => sum + point.tx, 0)

  return (
    <Card title={title} actions={
      <div role="group" aria-label="Rentang grafik" className="flex gap-1">
        {RANGES.map(([value, label]) => (
          <Button key={value} size="sm" variant={range === value ? 'primary' : 'outline'} aria-pressed={range === value}
            onClick={() => setRange(value)}>{label}</Button>
        ))}
      </div>
    }>
      {traffic.isError ? <p className="text-destructive">{traffic.error.message}</p> : (
        <>
          <p className="mb-3 text-sm text-muted-foreground">
            Terima <span className="font-mono text-foreground">{formatBytes(totalRx)}</span> · kirim{' '}
            <span className="font-mono text-foreground">{formatBytes(totalTx)}</span> (dari sisi server)
          </p>
          <figure aria-label={`Grafik trafik ${RANGES.find(([value]) => value === range)?.[1]}`} className="h-56 w-full" data-testid="traffic-chart">
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={points} margin={{ top: 4, right: 8, bottom: 0, left: 0 }}>
                <CartesianGrid stroke="var(--color-border)" vertical={false} />
                <XAxis dataKey="at" tickFormatter={(value: number) => chartLabel(value, range)} minTickGap={24}
                  stroke="var(--color-muted-foreground)" fontSize={12} />
                <YAxis tickFormatter={(value: number) => formatBytes(value)} width={72} stroke="var(--color-muted-foreground)" fontSize={12} />
                <Tooltip labelFormatter={(value) => chartLabel(Number(value), range)}
                  formatter={(value, name) => [formatBytes(Number(value)), name === 'rx' ? 'Terima' : 'Kirim']}
                  contentStyle={{ background: 'var(--color-card)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }} />
                <Area type="monotone" dataKey="rx" name="rx" stroke="var(--color-primary)" fill="var(--color-primary)" fillOpacity={0.15} isAnimationActive={false} />
                <Area type="monotone" dataKey="tx" name="tx" stroke="var(--color-online)" fill="var(--color-online)" fillOpacity={0.15} isAnimationActive={false} />
              </AreaChart>
            </ResponsiveContainer>
          </figure>
        </>
      )}
    </Card>
  )
}
