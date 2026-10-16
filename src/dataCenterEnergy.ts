/** Published U.S. data-center electricity estimates. Not a continuous annual census. */

export interface DataCenterEstimate {
  year: number;
  /** Terawatt-hours. One TWh equals one billion kWh. */
  twh: number;
  /** Share of U.S. electricity, when that study published one. */
  share: number | null;
  approximate: boolean;
  study: string;
  detail: string;
}

export const dataCenterEstimates: DataCenterEstimate[] = [
  {
    year: 2000,
    twh: 28.2,
    share: null,
    approximate: false,
    study: 'EPA and LBNL, 2007',
    detail:
      'Report to Congress, Figure 2-1. Includes servers, storage, network equipment, and the power and cooling infrastructure that supports them.',
  },
  {
    year: 2006,
    twh: 61.4,
    share: 1.5,
    approximate: false,
    study: 'EPA and LBNL, 2007',
    detail:
      'More than double the 2000 estimate, and about 1.5% of U.S. electricity consumption. The same table puts the 2000–2006 compound growth rate at 14% a year.',
  },
  {
    year: 2008,
    twh: 69,
    share: 1.8,
    approximate: false,
    study: 'Masanet et al., 2011',
    detail:
      'Cited by the 2016 LBNL report: 69 billion kWh, equal to 1.8% of U.S. electricity sales, using updated methods and inputs.',
  },
  {
    year: 2014,
    twh: 58,
    share: null,
    approximate: false,
    study: 'LBNL, 2024',
    detail:
      'The Department of Energy’s summary of the 2024 report: usage climbed from 58 TWh in 2014 to 176 TWh in 2023. The report itself describes 2014–2016 as about 60 TWh. The 2016 LBNL report had estimated 70 TWh for 2014, or 1.8% of U.S. electricity.',
  },
  {
    year: 2018,
    twh: 76,
    share: 1.9,
    approximate: true,
    study: 'LBNL, 2024',
    detail:
      'About 76 TWh, or 1.9% of U.S. electricity. The report says use started rising again in 2017 as the installed server base grew and GPU servers for AI became a meaningful part of the stock.',
  },
  {
    year: 2023,
    twh: 176,
    share: 4.4,
    approximate: false,
    study: 'LBNL, 2024',
    detail:
      '176 TWh, or 4.4% of U.S. electricity. GPU-accelerated AI servers grew from less than 2 TWh in 2017 to more than 40 TWh in 2023. Cryptocurrency mining is not included.',
  },
  {
    year: 2024,
    twh: 192,
    share: 4.7,
    approximate: false,
    study: 'LBNL, 2025 update',
    detail:
      '192 TWh, or 4.7% of U.S. electricity. The update puts prior years slightly below the 2024 report, mainly because reported GPU shipments for 2023 and 2024 were lower. This is the last year the update treats as historical.',
  },
];

/** End-use split published for 2000 and 2006 in the 2007 Report to Congress. */
export const earlyUseSplit = [
  { label: 'Volume servers', y2000: 8.0, y2006: 20.9 },
  { label: 'Mid-range servers', y2000: 2.5, y2006: 2.2 },
  { label: 'High-end servers', y2000: 1.1, y2006: 1.5 },
  { label: 'Storage', y2000: 1.1, y2006: 3.2 },
  { label: 'Network equipment', y2000: 1.4, y2006: 3.0 },
  { label: 'Power and cooling', y2000: 14.1, y2006: 30.7 },
] as const;

export const dataCenterSources = [
  {
    label: 'U.S. EPA and Lawrence Berkeley National Laboratory, Report to Congress on Server and Data Center Energy Efficiency (2007)',
    href: 'https://www.energystar.gov/sites/default/files/buildings/tools/Report%20to%20Congress%20on%20Server%20and%20Data%20Center%20Energy%20Efficiency.pdf',
  },
  {
    label: 'Shehabi et al., United States Data Center Energy Usage Report, Lawrence Berkeley National Laboratory (2016)',
    href: 'https://datacenters.lbl.gov/sites/default/files/DataCenterEnergyReport2016_0.pdf',
  },
  {
    label: 'Shehabi et al., 2024 United States Data Center Energy Usage Report, Lawrence Berkeley National Laboratory',
    href: 'https://eta-publications.lbl.gov/sites/default/files/2024-12/lbnl-2024-united-states-data-center-energy-usage-report.pdf',
  },
  {
    label: 'U.S. Department of Energy, announcement of the 2024 report (58 TWh in 2014, 176 TWh in 2023, 325–580 TWh in 2028)',
    href: 'https://www.energy.gov/articles/doe-releases-new-report-evaluating-increase-electricity-demand-data-centers',
  },
  {
    label: 'Lawrence Berkeley National Laboratory, United States Data Center Energy Usage Report: 2025 Update',
    href: 'https://www.energy.gov/documents/united-states-data-center-energy-usage-report-2025-update',
  },
] as const;
