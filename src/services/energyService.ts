export interface EnergyMix {
  period: string;
  coal: number;
  naturalGas: number;
  nuclear: number;
  hydro: number;
  wind: number;
  solar: number;
  petroleum: number;
  other: number;
}

export type FuelKey = Exclude<keyof EnergyMix, 'period'>;

export interface SourceTrend {
  /** Terawatt-hours. */
  generation: number;
  /** Percent of total generation. */
  share: number;
  /** 12-month average share; null for the first 11 months of data. */
  rollingShare: number | null;
  /** Percentage points vs. the same month a year earlier. */
  yoyChange: number | null;
}

export interface TrendPoint {
  period: string;
  sources: Partial<Record<FuelKey, SourceTrend>>;
}

export interface Forecast {
  /** YYYY-MM the forecast was retrieved. */
  vintage: string;
  points: TrendPoint[];
}

export interface Milestone {
  period: string;
  source: FuelKey;
  passedSource: FuelKey;
  share: number;
  description: string;
}

const apiUrl = '/api/energy/range';

export async function getEnergyData(
  start: string,
  end: string,
  signal?: AbortSignal
): Promise<EnergyMix[]> {
  const response = await fetch(`${apiUrl}?start=${start}&end=${end}`, { signal });
  if (!response.ok) {
    throw new Error(`Energy API request failed: ${response.status}`);
  }
  return response.json();
}

/** Resolves to null when the server's data source doesn't provide trends (HTTP 404). */
async function getOptional<T>(url: string, signal?: AbortSignal): Promise<T | null> {
  const response = await fetch(url, { signal });
  if (response.status === 404) {
    return null;
  }
  if (!response.ok) {
    throw new Error(`Energy API request failed: ${response.status}`);
  }
  return response.json();
}

export function getEnergyTrends(start: string, end: string, signal?: AbortSignal) {
  return getOptional<TrendPoint[]>(`/api/energy/trends?start=${start}&end=${end}`, signal);
}

export function getMilestones(signal?: AbortSignal) {
  return getOptional<Milestone[]>('/api/energy/milestones', signal);
}

export function getForecast(signal?: AbortSignal) {
  return getOptional<Forecast>('/api/energy/forecast', signal);
}
