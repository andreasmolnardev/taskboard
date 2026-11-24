import { useMemo } from 'react';
import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  Line,
  LineChart,
  XAxis,
  YAxis,
} from 'recharts';
import { ChartContainer, ChartTooltip, ChartTooltipContent, type ChartConfig } from '@/components/ui/chart';

export type DataChartSeries = {
  key: string;
  label: string;
  color: string;
};

export function DataChart({
  data,
  categoryKey,
  series,
  kind = 'line',
  height = 280,
}: {
  data: readonly Record<string, string | number>[];
  categoryKey: string;
  series: readonly DataChartSeries[];
  kind?: 'line' | 'bar' | 'area';
  height?: number;
}) {
  const config = useMemo<ChartConfig>(() => Object.fromEntries(
    series.map((item) => [item.key, { label: item.label, color: item.color }]),
  ), [series]);
  const chartData = useMemo(() => [...data], [data]);
  const axis = <><CartesianGrid vertical={false} /><XAxis dataKey={categoryKey} tickLine={false} axisLine={false} /><YAxis tickLine={false} axisLine={false} width={40} /></>;

  return (
    <ChartContainer config={config} className="w-full" style={{ height }}>
      {kind === 'bar' ? (
        <BarChart data={chartData} margin={{ left: 8, right: 8 }}>{axis}<ChartTooltip content={<ChartTooltipContent />} />{series.map((item) => <Bar key={item.key} dataKey={item.key} fill={`var(--color-${item.key})`} radius={[4, 4, 0, 0]} />)}</BarChart>
      ) : kind === 'area' ? (
        <AreaChart data={chartData} margin={{ left: 8, right: 8 }}>{axis}<ChartTooltip content={<ChartTooltipContent />} />{series.map((item) => <Area key={item.key} type="monotone" dataKey={item.key} stroke={`var(--color-${item.key})`} fill={`var(--color-${item.key})`} fillOpacity={0.18} />)}</AreaChart>
      ) : (
        <LineChart data={chartData} margin={{ left: 8, right: 8 }}>{axis}<ChartTooltip content={<ChartTooltipContent />} />{series.map((item) => <Line key={item.key} type="monotone" dataKey={item.key} stroke={`var(--color-${item.key})`} strokeWidth={2} dot={false} />)}</LineChart>
      )}
    </ChartContainer>
  );
}
