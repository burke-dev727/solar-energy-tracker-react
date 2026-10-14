import { useEffect, useState, type CSSProperties } from 'react';
import { ArcElement, Chart as ChartJS, Tooltip, type ChartData, type ChartOptions } from 'chart.js';
import { Pie } from 'react-chartjs-2';
import { formatPercent, formatPeriod, fuels, mixFromTrend } from './energy';
import {
  getEnergyData,
  getEnergyTrends,
  getForecast,
  getMilestones,
  type EnergyMix,
  type Forecast,
  type Milestone,
  type TrendPoint,
} from './services/energyService';
import { getChatEnabled } from './services/chatService';
import { ChatPanel } from './ChatPanel';
import { TrendsChart } from './TrendsChart';
import './App.css';

ChartJS.register(ArcElement, Tooltip);

const firstPeriod = '2001-01';
const playbackIntervalMs = 120;
const yearsBetweenTicks = 4;

const chartOptions: ChartOptions<'pie'> = {
  responsive: true,
  animation: { duration: 300 },
  layout: { padding: 16 },
  plugins: {
    tooltip: {
      backgroundColor: 'rgba(15, 23, 42, 0.95)',
      borderColor: 'rgba(148, 163, 184, 0.3)',
      borderWidth: 1,
      padding: 12,
      cornerRadius: 10,
      callbacks: {
        label: (context) => ` ${context.label}: ${formatPercent(context.parsed)}`,
      },
    },
  },
};

function currentPeriod() {
  const now = new Date();
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`;
}

function formatChange(points: number) {
  const rounded = Math.abs(points).toFixed(1);
  if (rounded === '0.0') {
    return '0.0 pts';
  }
  return `${points > 0 ? '▲' : '▼'} ${rounded} pts`;
}

function App() {
  const [selectedIndex, setSelectedIndex] = useState(0);
  const [energyData, setEnergyData] = useState<EnergyMix[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [playing, setPlaying] = useState(false);
  const [trends, setTrends] = useState<TrendPoint[]>([]);
  const [milestones, setMilestones] = useState<Milestone[]>([]);
  const [forecast, setForecast] = useState<Forecast | null>(null);
  const [chatEnabled, setChatEnabled] = useState(false);

  useEffect(() => {
    const controller = new AbortController();
    getChatEnabled(controller.signal)
      .then(setChatEnabled)
      .catch(() => setChatEnabled(false));
    return () => controller.abort();
  }, []);

  useEffect(() => {
    const controller = new AbortController();

    Promise.all([
      getEnergyTrends(firstPeriod, currentPeriod(), controller.signal),
      getMilestones(controller.signal),
      getForecast(controller.signal),
    ])
      .then(([trendData, milestoneData, forecastData]) => {
        setTrends(trendData ?? []);
        setMilestones(milestoneData ?? []);
        setForecast(forecastData);
      })
      .catch((error) => {
        if (!controller.signal.aborted) {
          console.error(error);
        }
      });

    return () => controller.abort();
  }, []);

  useEffect(() => {
    const controller = new AbortController();

    getEnergyData(firstPeriod, currentPeriod(), controller.signal)
      .then((data) => {
        setEnergyData(data);
        setSelectedIndex(Math.max(0, data.length - 1));
      })
      .catch((error) => {
        if (!controller.signal.aborted) {
          console.error(error);
          setError('Could not load energy data. Make sure the API server is running.');
        }
      });

    return () => controller.abort();
  }, []);

  const lastActualIndex = energyData.length - 1;
  const lastActualPeriod = energyData[lastActualIndex]?.period;
  const forecastPoints = lastActualPeriod
    ? (forecast?.points ?? []).filter((point) => point.period > lastActualPeriod)
    : [];
  const timeline = [...energyData, ...forecastPoints.map(mixFromTrend)];

  const lastIndex = timeline.length - 1;
  const atEnd = selectedIndex >= lastIndex;
  const isPlaying = playing && !atEnd;

  useEffect(() => {
    if (!isPlaying) {
      return;
    }
    const id = setInterval(() => {
      setSelectedIndex((index) => Math.min(index + 1, lastIndex));
    }, playbackIntervalMs);
    return () => clearInterval(id);
  }, [isPlaying, lastIndex]);

  const selected = timeline[selectedIndex];
  const isForecast = selectedIndex > lastActualIndex;

  if (error) {
    return (
      <main className="page">
        <div className="card status-card error">{error}</div>
      </main>
    );
  }

  if (!selected) {
    return (
      <main className="page">
        <div className="card status-card">
          <div className="spinner" />
          Loading energy data…
        </div>
      </main>
    );
  }

  const pieChartData: ChartData<'pie'> = {
    labels: fuels.map((fuel) => fuel.label),
    datasets: [
      {
        data: fuels.map((fuel) => selected[fuel.key]),
        backgroundColor: fuels.map((fuel) => fuel.color),
        borderColor: '#0f172a',
        borderWidth: 2,
        hoverOffset: 14,
      },
    ],
  };

  const stats = [
    { label: 'Fossil fuels', value: selected.coal + selected.naturalGas + selected.petroleum, color: '#f97316' },
    { label: 'Renewables', value: selected.hydro + selected.wind + selected.solar, color: '#2dd4bf' },
    { label: 'Nuclear', value: selected.nuclear, color: '#a855f7' },
  ];

  const firstYear = Number(timeline[0].period.slice(0, 4));
  const yearTicks = timeline.flatMap((mix, index) => {
    const year = Number(mix.period.slice(0, 4));
    return mix.period.endsWith('-01') && (year - firstYear) % yearsBetweenTicks === 0 ? [{ year, index }] : [];
  });

  const progress = lastIndex > 0 ? (selectedIndex / lastIndex) * 100 : 0;
  const forecastStart = forecastPoints.length > 0 ? (lastActualIndex + 0.5) / lastIndex : null;

  function togglePlayback() {
    if (isPlaying) {
      setPlaying(false);
      return;
    }
    if (atEnd) {
      setSelectedIndex(0);
    }
    setPlaying(true);
  }

  function selectMonth(index: number) {
    setPlaying(false);
    setSelectedIndex(Math.max(0, Math.min(index, lastIndex)));
  }

  function selectPeriod(period: string) {
    const index = timeline.findIndex((mix) => mix.period === period);
    if (index >= 0) {
      selectMonth(index);
    }
  }

  const selectedTrend = [...trends, ...forecastPoints].find((point) => point.period === selected.period);

  return (
    <main className="page">
      <header className="header">
        <span className="eyebrow">U.S. Energy Information Administration data</span>
        <h1>U.S. Electricity Mix</h1>
        <p className="subtitle">How America generated its electricity, month by month.</p>
      </header>

      <section className={isForecast ? 'card is-forecast' : 'card'}>
        <div className="period">
          <span className="period-label">{isForecast ? 'Projected' : 'Showing'}</span>
          <h2>{formatPeriod(selected.period)}</h2>
          {isForecast && forecast && (
            <span className="forecast-badge">
              EIA forecast · {formatPeriod(forecast.vintage, 'short')} Short-Term Energy Outlook
            </span>
          )}
        </div>

        <div className="stats">
          {stats.map((stat) => (
            <div key={stat.label} className="stat" style={{ '--accent': stat.color } as CSSProperties}>
              <span className="stat-value">{formatPercent(stat.value)}</span>
              <span className="stat-label">{stat.label}</span>
            </div>
          ))}
        </div>

        <div className="chart-layout">
          <div className="chart-container">
            <Pie data={pieChartData} options={chartOptions} />
          </div>

          <ul className="legend">
            {fuels.map((fuel) => {
              const yoyChange = selectedTrend?.sources[fuel.key]?.yoyChange;
              return (
                <li key={fuel.key} className="legend-item">
                  <span className="legend-swatch" style={{ background: fuel.color }} />
                  <span className="legend-label">{fuel.label}</span>
                  <span className="legend-yoy" title="Change in share vs. the same month a year earlier">
                    {yoyChange != null && formatChange(yoyChange)}
                  </span>
                  <span className="legend-value">{formatPercent(selected[fuel.key])}</span>
                  <span className="legend-bar">
                    <span
                      className="legend-bar-fill"
                      style={{ width: `${selected[fuel.key]}%`, background: fuel.color }}
                    />
                  </span>
                </li>
              );
            })}
          </ul>
        </div>
      </section>

      <section className="card timeline">
        <div className="timeline-controls">
          <button
            type="button"
            className="icon-button"
            onClick={() => selectMonth(selectedIndex - 1)}
            disabled={selectedIndex === 0}
            aria-label="Previous month"
          >
            ‹
          </button>
          <button
            type="button"
            className="play-button"
            onClick={togglePlayback}
            aria-label={isPlaying ? 'Pause' : 'Play'}
          >
            {isPlaying ? '❚❚' : '▶'}
          </button>
          <button
            type="button"
            className="icon-button"
            onClick={() => selectMonth(selectedIndex + 1)}
            disabled={atEnd}
            aria-label="Next month"
          >
            ›
          </button>
        </div>

        <div className="slider-area">
          <div className="slider-track">
            {forecastStart !== null && (
              <div className="forecast-zone" style={{ '--forecast-start': forecastStart } as CSSProperties} />
            )}
            <input
              className="slider"
              type="range"
              min="0"
              max={lastIndex}
              step="1"
              value={selectedIndex}
              onChange={(event) => selectMonth(Number(event.target.value))}
              style={{ '--progress': `${progress}%` } as CSSProperties}
              aria-label="Select month"
            />
          </div>
          <div className="ticks">
            {yearTicks.map((tick) => (
              <button
                key={tick.year}
                type="button"
                className="tick"
                style={{ left: `${(tick.index / lastIndex) * 100}%` }}
                onClick={() => selectMonth(tick.index)}
              >
                {tick.year}
              </button>
            ))}
          </div>
          <div className="range-labels">
            <span>{formatPeriod(timeline[0].period, 'short')}</span>
            <span>{formatPeriod(timeline[lastIndex].period, 'short')}</span>
          </div>
        </div>
      </section>

      {trends.length > 0 && (
        <TrendsChart
          trends={trends}
          forecast={forecastPoints}
          forecastVintage={forecast?.vintage}
          milestones={milestones}
          selectedPeriod={selected.period}
          onSelectPeriod={selectPeriod}
        />
      )}

      {chatEnabled && <ChatPanel view={{ period: selected.period, forecast: isForecast }} />}
    </main>
  );
}

export default App;
