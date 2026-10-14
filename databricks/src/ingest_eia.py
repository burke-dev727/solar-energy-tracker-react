# Databricks notebook source
# MAGIC %md
# MAGIC # Ingest EIA generation data (bronze)
# MAGIC
# MAGIC Pulls monthly U.S. electricity generation by fuel type from the EIA API and merges it
# MAGIC into the bronze table. Each run re-pulls the last few months because EIA revises
# MAGIC recent figures after first publishing them.

# COMMAND ----------

import json
from datetime import date, datetime, timezone

import requests

dbutils.widgets.text("catalog", "workspace")
dbutils.widgets.text("schema", "energy")
dbutils.widgets.text("secret_scope", "energy-tracker")
dbutils.widgets.text("initial_start", "2001-01")
dbutils.widgets.text("lookback_months", "6")

catalog = dbutils.widgets.get("catalog")
schema = dbutils.widgets.get("schema")
secret_scope = dbutils.widgets.get("secret_scope")
initial_start = dbutils.widgets.get("initial_start")
lookback_months = int(dbutils.widgets.get("lookback_months"))

bronze_table = f"`{catalog}`.`{schema}`.bronze_eia_generation"

EIA_URL = "https://api.eia.gov/v2/electricity/electric-power-operational-data/data/"
PAGE_LENGTH = 5000

# COMMAND ----------

spark.sql(f"CREATE SCHEMA IF NOT EXISTS `{catalog}`.`{schema}`")
spark.sql(
    f"""
    CREATE TABLE IF NOT EXISTS {bronze_table} (
      period STRING NOT NULL,
      fuel_type_id STRING NOT NULL,
      fuel_type_description STRING,
      generation STRING,
      generation_units STRING,
      raw_record STRING,
      ingested_at TIMESTAMP
    )
    COMMENT 'Raw EIA monthly U.S. generation records (all sectors), one row per period and fuel type.'
    """
)

# COMMAND ----------


def shift_month(period: str, months: int) -> str:
    year, month = map(int, period.split("-"))
    index = year * 12 + (month - 1) + months
    return f"{index // 12:04d}-{index % 12 + 1:02d}"


latest = spark.sql(f"SELECT max(period) AS latest FROM {bronze_table}").first()["latest"]
start = max(initial_start, shift_month(latest, -lookback_months)) if latest else initial_start
end = date.today().strftime("%Y-%m")
print(f"Fetching EIA data from {start} to {end}")

# COMMAND ----------


def fetch_records(api_key: str, start: str, end: str) -> list[dict]:
    records: list[dict] = []
    offset = 0
    while True:
        response = requests.get(
            EIA_URL,
            params={
                "api_key": api_key,
                "frequency": "monthly",
                "data[]": "generation",
                "facets[location][]": "US",
                "facets[sectorid][]": "98",
                "start": start,
                "end": end,
                "length": PAGE_LENGTH,
                "offset": offset,
            },
            timeout=60,
        )
        if response.status_code != 200:
            # The request URL contains the API key, so don't surface requests' own error message.
            raise RuntimeError(f"EIA API returned {response.status_code}")

        body = response.json()["response"]
        page = body["data"]
        records.extend(page)
        if not page or len(records) >= int(body["total"]):
            return records
        offset += PAGE_LENGTH


records = fetch_records(dbutils.secrets.get(secret_scope, "eia-api-key").strip(), start, end)
print(f"Fetched {len(records)} records")

# COMMAND ----------

def as_string(value) -> str | None:
    return None if value is None else str(value)


ingested_at = datetime.now(timezone.utc)
rows = {
    (record["period"], record["fueltypeid"]): (
        record["period"],
        record["fueltypeid"],
        record.get("fuelTypeDescription"),
        as_string(record.get("generation")),
        record.get("generation-units"),
        json.dumps(record),
        ingested_at,
    )
    for record in records
}

if rows:
    spark.createDataFrame(
        list(rows.values()),
        "period STRING, fuel_type_id STRING, fuel_type_description STRING, generation STRING, "
        "generation_units STRING, raw_record STRING, ingested_at TIMESTAMP",
    ).createOrReplaceTempView("eia_updates")

    spark.sql(
        f"""
        MERGE INTO {bronze_table} AS target
        USING eia_updates AS source
          ON target.period = source.period AND target.fuel_type_id = source.fuel_type_id
        WHEN MATCHED AND target.raw_record <> source.raw_record THEN UPDATE SET *
        WHEN NOT MATCHED THEN INSERT *
        """
    )

print(f"Merged {len(rows)} rows into {bronze_table}")
