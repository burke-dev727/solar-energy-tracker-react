import type { EnergyMix, FuelKey, TrendPoint } from './services/energyService';

export const fuels: { key: FuelKey; label: string; color: string }[] = [
  { key: 'coal', label: 'Coal', color: '#94a3b8' },
  { key: 'naturalGas', label: 'Natural Gas', color: '#f97316' },
  { key: 'nuclear', label: 'Nuclear', color: '#a855f7' },
  { key: 'hydro', label: 'Hydro', color: '#3b82f6' },
  { key: 'wind', label: 'Wind', color: '#2dd4bf' },
  { key: 'solar', label: 'Solar', color: '#facc15' },
  { key: 'petroleum', label: 'Petroleum', color: '#f43f5e' },
  { key: 'other', label: 'Other', color: '#a3e635' },
];

export function mixFromTrend(point: TrendPoint): EnergyMix {
  const share = (key: FuelKey) => point.sources[key]?.share ?? 0;
  return {
    period: point.period,
    coal: share('coal'),
    naturalGas: share('naturalGas'),
    nuclear: share('nuclear'),
    hydro: share('hydro'),
    wind: share('wind'),
    solar: share('solar'),
    petroleum: share('petroleum'),
    other: share('other'),
  };
}

export function fuelColor(key: FuelKey) {
  return fuels.find((fuel) => fuel.key === key)?.color ?? '#94a3b8';
}

export function formatPeriod(period: string, month: 'long' | 'short' = 'long') {
  const [year, monthNumber] = period.split('-').map(Number);
  return new Date(year, monthNumber - 1).toLocaleDateString('en-US', { month, year: 'numeric' });
}

export function formatPercent(value: number) {
  if (value > 0 && value < 0.1) {
    return '<0.1%';
  }
  return `${value.toFixed(1)}%`;
}
