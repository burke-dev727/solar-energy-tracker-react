# Databricks notebook source
# MAGIC %md
# MAGIC # Build trend and milestone tables
# MAGIC
# MAGIC * **gold_energy_trends** – per month and source: generation (TWh), share of total,
# MAGIC   12-month rolling share, and year-over-year change in share.
# MAGIC * **gold_energy_milestones** – the first month each source's 12-month rolling share
# MAGIC   overtook another source's and stayed ahead for at least a year.

# COMMAND ----------

dbutils.widgets.text("catalog", "workspace")
dbutils.widgets.text("schema", "energy")

catalog = dbutils.widgets.get("catalog")
schema = dbutils.widgets.get("schema")
prefix = f"`{catalog}`.`{schema}`"

SOURCES = [
    ("COW", "coal", "Coal"),
    ("NG", "natural_gas", "Natural gas"),
    ("NUC", "nuclear", "Nuclear"),
    ("HYC", "hydro", "Hydro"),
    ("WND", "wind", "Wind"),
    ("SUN", "solar", "Solar"),
    ("PET", "petroleum", "Petroleum"),
]

# Crossovers between very small sources (e.g. early wind vs. petroleum) are noise, so a
# milestone only counts once the overtaking source has at least this share.
MILESTONE_MIN_SHARE = 3.0

# A crossover only counts if the overtaking source started the data behind the other one
# (so a source regaining a lead it already had isn't reported) and stays ahead this many
# months (so brief back-and-forth swaps around the crossing point aren't reported).
MILESTONE_HOLD_MONTHS = 12

fuel_sources = ", ".join(f"('{fuel}', '{source}')" for fuel, source, _ in SOURCES)
source_names = ", ".join(f"('{source}', '{name}')" for _, source, name in SOURCES)

# COMMAND ----------

spark.sql(
    f"""
    CREATE OR REPLACE TABLE {prefix}.gold_energy_trends
    COMMENT 'Monthly generation and share of U.S. electricity by source, with 12-month rolling share and year-over-year change.'
    AS
    WITH fuel_sources AS (
      SELECT * FROM VALUES {fuel_sources} AS fuel_sources(fuel_type_id, source)
    ),
    totals AS (
      SELECT period, period_date, generation AS total
      FROM {prefix}.silver_generation_by_fuel
      WHERE fuel_type_id = 'ALL'
    ),
    named AS (
      SELECT t.period, t.period_date, f.source, coalesce(s.generation, 0) AS generation, t.total
      FROM totals t
      CROSS JOIN fuel_sources f
      LEFT JOIN {prefix}.silver_generation_by_fuel s
        ON s.period = t.period AND s.fuel_type_id = f.fuel_type_id
    ),
    with_other AS (
      SELECT period, period_date, source, generation, total FROM named
      UNION ALL
      SELECT period, period_date, 'other', total - sum(generation), total
      FROM named
      GROUP BY period, period_date, total
    ),
    windowed AS (
      SELECT
        *,
        sum(generation) OVER last_12_months AS generation_12mo,
        sum(total) OVER last_12_months AS total_12mo,
        min(period_date) OVER last_12_months AS window_start
      FROM with_other
      WHERE total <> 0
      WINDOW last_12_months AS (PARTITION BY source ORDER BY period_date ROWS BETWEEN 11 PRECEDING AND CURRENT ROW)
    )
    SELECT
      cur.period,
      cur.period_date,
      cur.source,
      round(cur.generation / 1000, 4) AS generation_twh,
      round(cur.generation / cur.total * 100, 3) AS share,
      CASE
        WHEN cur.window_start = add_months(cur.period_date, -11)
        THEN round(cur.generation_12mo / cur.total_12mo * 100, 3)
      END AS share_12mo_avg,
      round((cur.generation / cur.total - prev.generation / prev.total) * 100, 3) AS yoy_change_pts
    FROM windowed cur
    LEFT JOIN windowed prev
      ON prev.source = cur.source AND prev.period_date = add_months(cur.period_date, -12)
    """
)

# COMMAND ----------

unbalanced = spark.sql(
    f"""
    SELECT count(*) FROM (
      SELECT period FROM {prefix}.gold_energy_trends
      GROUP BY period HAVING abs(sum(share) - 100) > 0.1
    )
    """
).first()[0]
if unbalanced:
    raise ValueError(f"{unbalanced} periods have source shares that don't sum to 100%")

# COMMAND ----------

spark.sql(
    f"""
    CREATE OR REPLACE TABLE {prefix}.gold_energy_milestones
    COMMENT 'First month each source''s 12-month rolling share overtook another source''s and stayed ahead for a year.'
    AS
    WITH rolling AS (
      SELECT period, period_date, source, share_12mo_avg AS share
      FROM {prefix}.gold_energy_trends
      WHERE share_12mo_avg IS NOT NULL AND source <> 'other'
    ),
    pairs AS (
      SELECT
        a.period,
        a.period_date,
        a.source,
        b.source AS passed_source,
        a.share,
        b.share AS passed_share,
        lag(a.share) OVER pair_history AS previous_share,
        lag(b.share) OVER pair_history AS previous_passed_share,
        first_value(a.share - b.share) OVER pair_history AS initial_lead
      FROM rolling a
      JOIN rolling b ON a.period_date = b.period_date AND a.source <> b.source
      WINDOW pair_history AS (PARTITION BY a.source, b.source ORDER BY a.period_date)
    ),
    lookahead AS (
      SELECT
        *,
        min(share - passed_share) OVER next_months AS min_lead,
        count(*) OVER next_months AS months_observed
      FROM pairs
      WINDOW next_months AS (
        PARTITION BY source, passed_source ORDER BY period_date
        ROWS BETWEEN CURRENT ROW AND {MILESTONE_HOLD_MONTHS - 1} FOLLOWING
      )
    ),
    crossings AS (
      SELECT *, row_number() OVER (PARTITION BY source, passed_source ORDER BY period_date) AS occurrence
      FROM lookahead
      WHERE initial_lead < 0
        AND previous_share <= previous_passed_share
        AND share > passed_share
        AND share >= {MILESTONE_MIN_SHARE}
        AND months_observed = {MILESTONE_HOLD_MONTHS}
        AND min_lead > 0
    ),
    names AS (
      SELECT * FROM VALUES {source_names} AS names(source, name)
    )
    SELECT
      c.period,
      c.period_date,
      c.source,
      c.passed_source,
      c.share,
      c.passed_share,
      concat(n.name, ' passes ', lower(p.name)) AS description
    FROM crossings c
    JOIN names n ON n.source = c.source
    JOIN names p ON p.source = c.passed_source
    WHERE c.occurrence = 1
    """
)

# COMMAND ----------

for row in spark.sql(
    f"SELECT period, description FROM {prefix}.gold_energy_milestones ORDER BY period_date"
).collect():
    print(f"{row.period}: {row.description}")
