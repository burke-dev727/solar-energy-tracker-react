import { useState, type CSSProperties } from 'react';
import {
  BarElement,
  CategoryScale,
  Chart as ChartJS,
  Legend,
  LinearScale,
  LineElement,
  PointElement,
  Tooltip,
  type ChartData,
  type ChartOptions,
  type TooltipItem,
} from 'chart.js';
import { Bar, Line } from 'react-chartjs-2';
import { dataCenterEstimates, dataCenterSources, earlyUseSplit } from './dataCenterEnergy';

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, BarElement, Tooltip, Legend);

const baseline = dataCenterEstimates[0];
const tooltipStyle = {
  backgroundColor: 'rgba(15, 23, 42, 0.95)',
  borderColor: 'rgba(148, 163, 184, 0.3)',
  borderWidth: 1,
  padding: 12,
  cornerRadius: 10,
};

function formatTwh(value: number) {
  return Number.isInteger(value) ? String(value) : value.toFixed(1);
}

function nearestEstimate(year: number) {
  return dataCenterEstimates.reduce((closest, estimate) =>
    Math.abs(estimate.year - year) < Math.abs(closest.year - year) ? estimate : closest,
  );
}

export function DataCentersPage() {
  const [selectedYear, setSelectedYear] = useState(dataCenterEstimates[dataCenterEstimates.length - 1].year);
  const selected = dataCenterEstimates.find((estimate) => estimate.year === selectedYear) ?? baseline;
  const versusBaseline = selected.twh / baseline.twh;

  const lineData: ChartData<'line'> = {
    datasets: [
      {
        label: 'Published estimate',
        data: dataCenterEstimates.map((estimate) => ({ x: estimate.year, y: estimate.twh })),
        borderColor: '#facc15',
        backgroundColor: '#facc15',
        borderWidth: 2.5,
        tension: 0,
        pointRadius: (context) => (context.parsed.x === selected.year ? 8 : 5),
        pointHoverRadius: 8,
        pointBorderWidth: 2,
        pointBorderColor: '#0f172a',
        pointBackgroundColor: (context) => (context.parsed.x === selected.year ? '#ffffff' : '#facc15'),
      },
    ],
  };

  const lineOptions: ChartOptions<'line'> = {
    responsive: true,
    maintainAspectRatio: false,
    animation: false,
    layout: { padding: { right: 16, left: 8 } },
    onClick: (event, _elements, chart) => {
      if (event.x == null) {
        return;
      }
      const year = chart.scales.x.getValueForPixel(event.x);
      if (year != null) {
        setSelectedYear(nearestEstimate(year).year);
      }
    },
    scales: {
      x: {
        type: 'linear',
        min: 1999,
        max: 2025,
        grid: { display: false },
        ticks: {
          color: '#94a3b8',
          maxTicksLimit: 4,
          maxRotation: 0,
          callback: (value) => String(Math.round(Number(value))),
        },
      },
      y: {
        min: 0,
        grid: { color: 'rgba(148, 163, 184, 0.12)' },
        ticks: {
          color: '#94a3b8',
          callback: (value) => `${value}`,
        },
        title: {
          display: true,
          text: 'Terawatt-hours',
          color: '#94a3b8',
        },
      },
    },
    plugins: {
      legend: { display: false },
      tooltip: {
        ...tooltipStyle,
        callbacks: {
          title: (items) => {
            const year = items[0]?.parsed.x;
            return year == null ? '' : String(year);
          },
          label: (item: TooltipItem<'line'>) => {
            const estimate = dataCenterEstimates.find((entry) => entry.year === item.parsed.x);
            const value = `${formatTwh(item.parsed.y ?? 0)} TWh`;
            return estimate ? `${estimate.approximate ? 'About ' : ''}${value} · ${estimate.study}` : value;
          },
        },
      },
    },
  };

  const splitFocus = selected.year === 2000 || selected.year === 2006 ? selected.year : null;
  const barData: ChartData<'bar'> = {
    labels: earlyUseSplit.map((row) => row.label),
    datasets: [
      {
        label: '2000',
        data: earlyUseSplit.map((row) => row.y2000),
        backgroundColor: splitFocus === 2006 ? 'rgba(148, 163, 184, 0.28)' : '#38bdf8',
        borderRadius: 6,
      },
      {
        label: '2006',
        data: earlyUseSplit.map((row) => row.y2006),
        backgroundColor: splitFocus === 2000 ? 'rgba(250, 204, 21, 0.28)' : '#facc15',
        borderRadius: 6,
      },
    ],
  };

  const barOptions: ChartOptions<'bar'> = {
    indexAxis: 'y',
    responsive: true,
    maintainAspectRatio: false,
    animation: false,
    scales: {
      x: {
        min: 0,
        grid: { color: 'rgba(148, 163, 184, 0.12)' },
        ticks: { color: '#94a3b8', callback: (value) => `${value}` },
        title: { display: true, text: 'Terawatt-hours', color: '#94a3b8' },
      },
      y: {
        grid: { display: false },
        ticks: { color: '#94a3b8' },
      },
    },
    plugins: {
      legend: {
        labels: { color: '#f1f5f9', boxWidth: 12, boxHeight: 12 },
      },
      tooltip: {
        ...tooltipStyle,
        callbacks: {
          label: (item) => ` ${item.dataset.label}: ${formatTwh(item.parsed.y ?? 0)} TWh`,
        },
      },
    },
  };

  return (
    <main className="page">
      <header className="header">
        <span className="eyebrow">Lawrence Berkeley National Laboratory and EPA estimates</span>
        <h1>U.S. Data Centers</h1>
        <p className="subtitle">
          Electricity used by American data centers, from the federal studies that estimate the series back to 2000.
        </p>
      </header>

      <section className="card">
        <div className="period">
          <span className="period-label">Estimate for {selected.year}</span>
          <h2>
            {selected.approximate ? '≈ ' : ''}
            {formatTwh(selected.twh)} TWh
          </h2>
          <span className="forecast-badge">{selected.study}</span>
        </div>

        <div className="stats">
          <div className="stat" style={{ '--accent': '#facc15' } as CSSProperties}>
            <span className="stat-label">Versus 2000</span>
            <span className="stat-value">{versusBaseline.toFixed(1)}×</span>
          </div>
          <div className="stat" style={{ '--accent': '#38bdf8' } as CSSProperties}>
            <span className="stat-label">Share of U.S. electricity</span>
            <span className="stat-value">{selected.share == null ? '—' : `${selected.share.toFixed(1)}%`}</span>
          </div>
          <div className="stat" style={{ '--accent': '#f97316' } as CSSProperties}>
            <span className="stat-label">Change from prior estimate</span>
            <span className="stat-value">{priorChange(selected.year)}</span>
          </div>
        </div>

        <p className="dc-detail">{selected.detail}</p>

        <div className="dc-chart">
          <Line data={lineData} options={lineOptions} />
        </div>

        <div className="dc-years" role="group" aria-label="Estimate years">
          {dataCenterEstimates.map((estimate) => (
            <button
              key={estimate.year}
              type="button"
              className={estimate.year === selected.year ? 'active' : undefined}
              aria-pressed={estimate.year === selected.year}
              onClick={() => setSelectedYear(estimate.year)}
            >
              {estimate.year}
            </button>
          ))}
        </div>
        <p className="dc-note">
          Each point is a published estimate. The straight segments between them are not annual measurements, and later
          reports revised some earlier years. One terawatt-hour equals one billion kilowatt-hours.
        </p>
      </section>

      <section className="card">
        <h2 className="dc-title">Where the electricity went in 2000 and 2006</h2>
        <p className="trends-subtitle">
          The 2007 Report to Congress is the study that splits those two years into equipment and infrastructure. Power
          and cooling was half of the total in both years. Volume servers were the piece that grew fastest.
        </p>
        <div className="dc-chart dc-chart-bar">
          <Bar data={barData} options={barOptions} />
        </div>
      </section>

      <section className="card is-forecast">
        <h2 className="dc-title">What the same studies project next</h2>
        <p className="dc-detail">
          These are scenarios, not history. The 2024 report put 2028 between 325 and 580 TWh, about 6.7% to 12% of U.S.
          electricity. The 2025 update’s reference case is 464 TWh in 2028 and 649 TWh in 2030 (11.8%), inside a
          compounded range of 521 to 843 TWh in 2030. Cryptocurrency mining is outside the scope of both reports.
        </p>
      </section>

      <section className="card">
        <h2 className="dc-title">Sources</h2>
        <ul className="dc-sources">
          {dataCenterSources.map((source) => (
            <li key={source.href}>
              <a href={source.href} target="_blank" rel="noreferrer">
                {source.label}
              </a>
            </li>
          ))}
        </ul>
      </section>
    </main>
  );
}

function priorChange(year: number) {
  const index = dataCenterEstimates.findIndex((estimate) => estimate.year === year);
  if (index <= 0) {
    return '—';
  }
  const current = dataCenterEstimates[index];
  const previous = dataCenterEstimates[index - 1];
  const change = ((current.twh - previous.twh) / previous.twh) * 100;
  const rounded = Math.abs(change).toFixed(0);
  return `${change > 0 ? '+' : '−'}${rounded}%`;
}
