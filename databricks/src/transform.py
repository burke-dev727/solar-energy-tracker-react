# Databricks notebook source
# MAGIC %md
# MAGIC # Build silver and gold energy tables
# MAGIC
# MAGIC * **Silver** – typed, validated generation by period and fuel type.
# MAGIC * **Gold** – monthly energy mix percentages, shaped like the API's `EnergyMix` response.
# MAGIC
# MAGIC Both tables are rebuilt from bronze on every run; the dataset is small enough that
# MAGIC a full rebuild is simpler and safer than incremental updates.

# COMMAND ----------

dbutils.widgets.text("catalog", "workspace")
dbutils.widgets.text("schema", "energy")

catalog = dbutils.widgets.get("catalog")
schema = dbutils.widgets.get("schema")
prefix = f"`{catalog}`.`{schema}`"

# COMMAND ----------

spark.sql(
    f"""
    CREATE OR REPLACE TABLE {prefix}.silver_generation_by_fuel
    COMMENT 'Monthly U.S. net generation by fuel type, in thousand megawatt hours.'
    AS SELECT
      period,
      to_date(try_to_timestamp(period, 'yyyy-MM')) AS period_date,
      fuel_type_id,
      fuel_type_description,
      try_cast(generation AS DOUBLE) AS generation,
      generation_units
    FROM {prefix}.bronze_eia_generation
    WHERE generation IS NOT NULL
    """
)

# COMMAND ----------

checks = {
    "rows with an invalid period": f"""
        SELECT count(*) FROM {prefix}.silver_generation_by_fuel WHERE period_date IS NULL
    """,
    "rows with a non-numeric generation value": f"""
        SELECT count(*) FROM {prefix}.silver_generation_by_fuel WHERE generation IS NULL
    """,
    "periods without an ALL (total) row": f"""
        SELECT count(*) FROM (
          SELECT period FROM {prefix}.silver_generation_by_fuel
          GROUP BY period HAVING count_if(fuel_type_id = 'ALL') = 0
        )
    """,
}

failures = {name: count for name, query in checks.items() if (count := spark.sql(query).first()[0]) > 0}
if failures:
    raise ValueError(f"Silver data quality checks failed: {failures}")

# COMMAND ----------

spark.sql(
    f"""
    CREATE OR REPLACE TABLE {prefix}.gold_energy_mix_monthly
    COMMENT 'Share of U.S. electricity generation by source per month, in percent.'
    AS
    WITH generation AS (
      SELECT
        period,
        period_date,
        sum(CASE WHEN fuel_type_id = 'ALL' THEN generation END) AS total,
        coalesce(sum(CASE WHEN fuel_type_id = 'COW' THEN generation END), 0) AS coal,
        coalesce(sum(CASE WHEN fuel_type_id = 'NG'  THEN generation END), 0) AS natural_gas,
        coalesce(sum(CASE WHEN fuel_type_id = 'NUC' THEN generation END), 0) AS nuclear,
        coalesce(sum(CASE WHEN fuel_type_id = 'HYC' THEN generation END), 0) AS hydro,
        coalesce(sum(CASE WHEN fuel_type_id = 'WND' THEN generation END), 0) AS wind,
        coalesce(sum(CASE WHEN fuel_type_id = 'SUN' THEN generation END), 0) AS solar,
        coalesce(sum(CASE WHEN fuel_type_id = 'PET' THEN generation END), 0) AS petroleum
      FROM {prefix}.silver_generation_by_fuel
      GROUP BY period, period_date
    ),
    shares AS (
      SELECT
        *,
        total - (coal + natural_gas + nuclear + hydro + wind + solar + petroleum) AS other
      FROM generation
    )
    SELECT
      period,
      period_date,
      round(coal        / total * 100, 3) AS coal,
      round(natural_gas / total * 100, 3) AS natural_gas,
      round(nuclear     / total * 100, 3) AS nuclear,
      round(hydro       / total * 100, 3) AS hydro,
      round(wind        / total * 100, 3) AS wind,
      round(solar       / total * 100, 3) AS solar,
      round(petroleum   / total * 100, 3) AS petroleum,
      round(other       / total * 100, 3) AS other,
      round((hydro + wind + solar) / total * 100, 3) AS renewables_share,
      total AS total_generation,
      current_timestamp() AS updated_at
    FROM shares
    WHERE total <> 0
    """
)

# COMMAND ----------

summary = spark.sql(
    f"SELECT count(*) AS months, min(period) AS first, max(period) AS last FROM {prefix}.gold_energy_mix_monthly"
).first()
print(f"gold_energy_mix_monthly: {summary.months} months ({summary.first} to {summary.last})")
