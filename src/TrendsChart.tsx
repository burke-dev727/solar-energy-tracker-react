import { useState } from 'react';
import {
  CategoryScale,
  Chart as ChartJS,
  Filler,
  LinearScale,
  LineElement,
  PointElement,
  Tooltip,
  type ChartData,
  type ChartOptions,
  type ChartType,
  type Plugin,
} from 'chart.js';
import { Line } from 'react-chartjs-2';
import { formatPeriod, fuelColor, fuels } from './energy';
import type { Milestone, TrendPoint } from './services/energyService';

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Filler, Tooltip);

declare module 'chart.js' {
  interface PluginOptionsByType<TType extends ChartType> {
    monthMarkers?: { selectedIndex: number; milestoneIndexes: number[]; forecastStartIndex: number };
  }
}

const yearsBetweenLabels = 5;

const monthMarkersPlugin: Plugin<'line'> = {
  id: 'monthMarkers',
  afterDatasetsDraw(chart) {
    const markers = chart.options.plugins?.monthMarkers;
    const selectedIndex = markers?.selectedIndex ?? -1;
    const milestoneIndexes = markers?.milestoneIndexes ?? [];
    const forecastStartIndex = markers?.forecastStartIndex ?? -1;
    const { ctx, chartArea, scales } = chart;

    if (forecastStartIndex > 0) {
      const x =
        (scales.x.getPixelForValue(forecastStartIndex - 1) + scales.x.getPixelForValue(forecastStartIndex)) / 2;
      ctx.save();
      ctx.fillStyle = 'rgba(11, 17, 32, 0.45)';
      ctx.fillRect(x, chartArea.top, chartArea.right - x, chartArea.bottom - chartArea.top);
      ctx.setLineDash([4, 4]);
      ctx.strokeStyle = 'rgba(253, 230, 138, 0.8)';
      ctx.beginPath();
      ctx.moveTo(x, chartArea.top);
      ctx.lineTo(x, chartArea.bottom);
      ctx.stroke();
      ctx.fillStyle = '#fde68a';
      ctx.font = '600 11px Inter, "Segoe UI", system-ui, sans-serif';
      ctx.textAlign = 'right';
      ctx.fillText('EIA FORECAST', chartArea.right - 6, chartArea.top + 14);
      ctx.restore();
    }

    const drawLine = (index: number) => {
      const x = scales.x.getPixelForValue(index);
      ctx.beginPath();
      ctx.moveTo(x, chartArea.top);
      ctx.lineTo(x, chartArea.bottom);
      ctx.stroke();
      return x;
    };

    ctx.save();
    ctx.setLineDash([3, 4]);
    ctx.lineWidth = 1;
    ctx.strokeStyle = 'rgba(253, 230, 138, 0.55)';
    for (const index of milestoneIndexes) {
      if (index != null) {
        drawLine(index);
      }
    }

    if (selectedIndex >= 0) {
      ctx.setLineDash([]);
      ctx.lineWidth = 2;
      ctx.strokeStyle = '#ffffff';
      const x = drawLine(selectedIndex);
      ctx.fillStyle = '#ffffff';
      ctx.beginPath();
      ctx.arc(x, chartArea.top, 4, 0, Math.PI * 2);
      ctx.fill();
    }
    ctx.restore();
  },
};

type Metric = 'share' | 'generation';

interface TrendsChartProps {
  trends: TrendPoint[];
  /** Months after the latest actual data, drawn as a forecast. */
  forecast: TrendPoint[];
  forecastVintage?: string;
  milestones: Milestone[];
  selectedPeriod: string;
  onSelectPeriod: (period: string) => void;
}

export function TrendsChart({
  trends,
  forecast,
  forecastVintage,
  milestones,
  selectedPeriod,
  onSelectPeriod,
}: TrendsChartProps) {
  const [metric, setMetric] = useState<Metric>('share');
  const [smoothed, setSmoothed] = useState(true);

  const useRolling = metric === 'share' && smoothed;
  const points = [...trends, ...forecast];
  const forecastStartIndex = forecast.length > 0 ? trends.length : -1;
  const isForecastIndex = (index: number) => forecastStartIndex >= 0 && index >= forecastStartIndex;
  const labels = points.map((point) => point.period);
  const indexByPeriod = new Map(labels.map((period, index) => [period, index]));
  const formatValue = (value: number) => (metric === 'share' ? `${value.toFixed(1)}%` : `${value.toFixed(1)} TWh`);

  const data: ChartData<'line'> = {
    labels,
    datasets: fuels.map((fuel, index) => ({
      label: fuel.label,
      data: points.map((point) => {
        const source = point.sources[fuel.key];
        if (!source) {
          return null;
        }
        if (metric === 'generation') {
          return source.generation;
        }
        return useRolling ? source.rollingShare : source.share;
      }),
      borderColor: fuel.color,
      backgroundColor: `${fuel.color}b3`,
      borderWidth: 1,
      pointRadius: 0,
      pointHoverRadius: 3,
      tension: 0.25,
      fill: index === 0 ? 'origin' : '-1',
      segment: {
        borderDash: (context) => (isForecastIndex(context.p1DataIndex) ? [5, 4] : undefined),
      },
    })),
  };

  const options: ChartOptions<'line'> = {
    responsive: true,
    maintainAspectRatio: false,
    animation: false,
    interaction: { mode: 'index', intersect: false },
    onClick: (event, _elements, chart) => {
      if (event.x == null) {
        return;
      }
      const period = labels[Math.round(chart.scales.x.getValueForPixel(event.x) ?? -1)];
      if (period) {
        onSelectPeriod(period);
      }
    },
    scales: {
      x: {
        grid: { display: false },
        ticks: {
          color: '#94a3b8',
          autoSkip: false,
          maxRotation: 0,
          callback: (_value, index) => {
            const period = labels[index];
            const year = Number(period?.slice(0, 4));
            return period?.endsWith('-01') && year % yearsBetweenLabels === 0 ? String(year) : '';
          },
        },
      },
      y: {
        stacked: true,
        min: 0,
        max: metric === 'share' ? 100 : undefined,
        grid: { color: 'rgba(148, 163, 184, 0.12)' },
        ticks: {
          color: '#94a3b8',
          callback: (value) => (metric === 'share' ? `${value}%` : `${value} TWh`),
        },
      },
    },
    plugins: {
      tooltip: {
        backgroundColor: 'rgba(15, 23, 42, 0.95)',
        borderColor: 'rgba(148, 163, 184, 0.3)',
        borderWidth: 1,
        padding: 12,
        cornerRadius: 10,
        itemSort: (a, b) => b.datasetIndex - a.datasetIndex,
        callbacks: {
          title: (items) => {
            if (!items.length) {
              return '';
            }
            const index = items[0].dataIndex;
            return `${formatPeriod(labels[index])}${isForecastIndex(index) ? ' (EIA forecast)' : ''}`;
          },
          label: (context) =>
            context.parsed.y == null ? '' : ` ${context.dataset.label}: ${formatValue(context.parsed.y)}`,
        },
      },
      monthMarkers: {
        selectedIndex: indexByPeriod.get(selectedPeriod) ?? -1,
        milestoneIndexes: milestones.flatMap((milestone) => indexByPeriod.get(milestone.period) ?? []),
        forecastStartIndex,
      },
    },
  };

  const subtitle =
    metric === 'generation'
      ? 'Monthly generation in terawatt-hours'
      : useRolling
        ? 'Share of generation, 12-month average'
        : 'Share of generation, monthly';

  return (
    <section className="card trends">
      <div className="trends-header">
        <div>
          <h3>Trends over time</h3>
          <p className="trends-subtitle">{subtitle}</p>
        </div>
        <div className="toggles">
          <div className="segmented" role="group" aria-label="Metric">
            <button type="button" className={metric === 'share' ? 'active' : ''} onClick={() => setMetric('share')}>
              Share
            </button>
            <button
              type="button"
              className={metric === 'generation' ? 'active' : ''}
              onClick={() => setMetric('generation')}
            >
              TWh
            </button>
          </div>
          <div className="segmented" role="group" aria-label="Smoothing">
            <button
              type="button"
              className={useRolling ? 'active' : ''}
              disabled={metric !== 'share'}
              onClick={() => setSmoothed(true)}
            >
              12-mo avg
            </button>
            <button
              type="button"
              className={useRolling ? '' : 'active'}
              disabled={metric !== 'share'}
              onClick={() => setSmoothed(false)}
            >
              Monthly
            </button>
          </div>
        </div>
      </div>

      <div className="trends-chart">
        <Line data={data} options={options} plugins={[monthMarkersPlugin]} />
      </div>

      {milestones.length > 0 && (
        <div className="milestones">
          <span className="milestones-label">Milestones</span>
          <ul>
            {milestones.map((milestone) => (
              <li key={`${milestone.period}-${milestone.source}-${milestone.passedSource}`}>
                <button
                  type="button"
                  className={milestone.period === selectedPeriod ? 'milestone active' : 'milestone'}
                  onClick={() => onSelectPeriod(milestone.period)}
                >
                  <span className="milestone-dot" style={{ background: fuelColor(milestone.source) }} />
                  <span className="milestone-date">{formatPeriod(milestone.period, 'short')}</span>
                  {milestone.description}
                </button>
              </li>
            ))}
          </ul>
        </div>
      )}

      <p className="trends-note">
        Milestones mark when a source's 12-month average share overtook one that led it in 2001 and then stayed ahead
        for at least a year. Click the chart or a milestone to jump to that month.
        {forecastVintage && forecast.length > 0 && (
          <>
            {' '}
            Dashed lines show EIA's {formatPeriod(forecastVintage)} Short-Term Energy Outlook through{' '}
            {formatPeriod(forecast[forecast.length - 1].period)}.
          </>
        )}
      </p>
    </section>
  );
}
