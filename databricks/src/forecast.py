# Databricks notebook source
# MAGIC %md
# MAGIC # Build EIA forecast tables
# MAGIC
# MAGIC * **silver_steo_generation** – every STEO vintage, typed, per period and source (TWh).
# MAGIC * **gold_energy_forecast** – months after the latest actual data from the newest vintage,
# MAGIC   shaped like `gold_energy_trends`. Rolling averages and year-over-year changes are
# MAGIC   computed across actual and forecast months together so they continue smoothly.

# COMMAND ----------

dbutils.widgets.text("catalog", "workspace")
dbutils.widgets.text("schema", "energy")

catalog = dbutils.widgets.get("catalog")
schema = dbutils.widgets.get("schema")
prefix = f"`{catalog}`.`{schema}`"

TOTAL_SERIES = "TOEPGEN_US"
SERIES_SOURCES = [
    ("CLEPGEN_US", "coal"),
    ("NGEPGEN_US", "natural_gas"),
    ("NUEPGEN_US", "nuclear"),
    ("HVEPGEN_US", "hydro"),
    ("WNEPGEN_US", "wind"),
    ("SOEPGEN_US", "solar"),
    ("PAEPGEN_US", "petroleum"),
]
EXPECTED_UNIT = "billion kilowatthours"

# STEO's historical months should match the actuals; a larger gap means the definitions
# have drifted and the forecast would no longer line up with the actual data.
MAX_HISTORY_DIFFERENCE_PCT = 5.0

series_sources = ", ".join(f"('{series}', '{source}')" for series, source in SERIES_SOURCES)

# COMMAND ----------

unexpected_units = spark.sql(
    f"""
    SELECT DISTINCT unit FROM {prefix}.bronze_steo_generation
    WHERE unit IS DISTINCT FROM '{EXPECTED_UNIT}'
    """
).collect()
if unexpected_units:
    raise ValueError(f"Unexpected STEO units: {[row.unit for row in unexpected_units]}")

# COMMAND ----------

spark.sql(
    f"""
    CREATE OR REPLACE TABLE {prefix}.silver_steo_generation
    COMMENT 'EIA STEO electric power sector generation by vintage, period, and source, in TWh (billion kWh).'
    AS
    WITH series_sources AS (
      SELECT * FROM VALUES {series_sources} AS series_sources(series_id, source)
    ),
    typed AS (
      SELECT
        vintage,
        period,
        to_date(try_to_timestamp(period, 'yyyy-MM')) AS period_date,
        series_id,
        try_cast(value AS DOUBLE) AS generation_twh
      FROM {prefix}.bronze_steo_generation
    ),
    totals AS (
      SELECT vintage, period, period_date, generation_twh AS total_twh
      FROM typed
      WHERE series_id = '{TOTAL_SERIES}' AND generation_twh IS NOT NULL
    ),
    named AS (
      SELECT t.vintage, t.period, t.period_date, s.source, coalesce(g.generation_twh, 0) AS generation_twh, t.total_twh
      FROM totals t
      CROSS JOIN series_sources s
      LEFT JOIN typed g
        ON g.vintage = t.vintage AND g.period = t.period AND g.series_id = s.series_id
    )
    SELECT vintage, period, period_date, source, generation_twh, total_twh FROM named
    UNION ALL
    SELECT vintage, period, period_date, 'other', total_twh - sum(generation_twh), total_twh
    FROM named
    GROUP BY vintage, period, period_date, total_twh
    """
)

# COMMAND ----------

comparison = spark.sql(
    f"""
    WITH latest AS (SELECT max(vintage) AS vintage FROM {prefix}.silver_steo_generation),
    steo AS (
      SELECT s.period, sum(s.generation_twh) AS total_twh
      FROM {prefix}.silver_steo_generation s JOIN latest l ON s.vintage = l.vintage
      GROUP BY s.period
    ),
    actual AS (
      SELECT period, sum(generation_twh) AS total_twh
      FROM {prefix}.gold_energy_trends
      GROUP BY period
    )
    SELECT count(*) AS months, max(abs(steo.total_twh - actual.total_twh) / actual.total_twh * 100) AS max_difference_pct
    FROM steo JOIN actual ON steo.period = actual.period
    """
).first()

print(
    f"STEO history vs. actuals: {comparison.months} overlapping months, "
    f"max total difference {comparison.max_difference_pct or 0:.3f}%"
)
if comparison.months and comparison.max_difference_pct > MAX_HISTORY_DIFFERENCE_PCT:
    raise ValueError(
        f"STEO history differs from actuals by up to {comparison.max_difference_pct:.1f}%; "
        "check that the series still cover the electric power sector"
    )

# COMMAND ----------

spark.sql(
    f"""
    CREATE OR REPLACE TABLE {prefix}.gold_energy_forecast
    COMMENT 'EIA STEO forecast of U.S. electric power sector generation by source for months after the latest actual data.'
    AS
    WITH latest AS (
      SELECT max(vintage) AS vintage FROM {prefix}.silver_steo_generation
    ),
    last_actual AS (
      SELECT max(period_date) AS period_date FROM {prefix}.gold_energy_trends
    ),
    actual AS (
      SELECT
        period,
        period_date,
        source,
        generation_twh,
        sum(generation_twh) OVER (PARTITION BY period) AS total_twh,
        false AS is_forecast
      FROM {prefix}.gold_energy_trends
    ),
    forecast AS (
      SELECT s.period, s.period_date, s.source, s.generation_twh, s.total_twh, true AS is_forecast
      FROM {prefix}.silver_steo_generation s
      JOIN latest l ON s.vintage = l.vintage
      CROSS JOIN last_actual a
      WHERE s.period_date > a.period_date
    ),
    windowed AS (
      SELECT
        *,
        sum(generation_twh) OVER last_12_months AS generation_12mo,
        sum(total_twh) OVER last_12_months AS total_12mo,
        min(period_date) OVER last_12_months AS window_start
      FROM (SELECT * FROM actual UNION ALL SELECT * FROM forecast)
      WHERE total_twh <> 0
      WINDOW last_12_months AS (PARTITION BY source ORDER BY period_date ROWS BETWEEN 11 PRECEDING AND CURRENT ROW)
    )
    SELECT
      l.vintage,
      cur.period,
      cur.period_date,
      cur.source,
      round(cur.generation_twh, 4) AS generation_twh,
      round(cur.generation_twh / cur.total_twh * 100, 3) AS share,
      CASE
        WHEN cur.window_start = add_months(cur.period_date, -11)
        THEN round(cur.generation_12mo / cur.total_12mo * 100, 3)
      END AS share_12mo_avg,
      round((cur.generation_twh / cur.total_twh - prev.generation_twh / prev.total_twh) * 100, 3) AS yoy_change_pts
    FROM windowed cur
    CROSS JOIN latest l
    LEFT JOIN windowed prev
      ON prev.source = cur.source AND prev.period_date = add_months(cur.period_date, -12)
    WHERE cur.is_forecast
    """
)

# COMMAND ----------

unbalanced = spark.sql(
    f"""
    SELECT count(*) FROM (
      SELECT period FROM {prefix}.gold_energy_forecast
      GROUP BY period HAVING abs(sum(share) - 100) > 0.1
    )
    """
).first()[0]
if unbalanced:
    raise ValueError(f"{unbalanced} forecast periods have source shares that don't sum to 100%")

summary = spark.sql(
    f"""
    SELECT min(vintage) AS vintage, count(DISTINCT period) AS months, min(period) AS first, max(period) AS last
    FROM {prefix}.gold_energy_forecast
    """
).first()
print(f"gold_energy_forecast: vintage {summary.vintage}, {summary.months} months ({summary.first} to {summary.last})")
