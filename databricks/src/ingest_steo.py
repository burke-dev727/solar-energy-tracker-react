# Databricks notebook source
# MAGIC %md
# MAGIC # Ingest EIA Short-Term Energy Outlook (bronze)
# MAGIC
# MAGIC Snapshots the current STEO electricity generation series for the electric power sector,
# MAGIC the same sector as the actuals. STEO is republished monthly and the API only serves the
# MAGIC latest release, so each run is stored under the month it was retrieved (its "vintage").
# MAGIC Over time this builds a history of forecasts that can be compared with what happened.

# COMMAND ----------

import json
from datetime import date, datetime, timezone

import requests

dbutils.widgets.text("catalog", "workspace")
dbutils.widgets.text("schema", "energy")
dbutils.widgets.text("secret_scope", "energy-tracker")
dbutils.widgets.text("history_months", "24")

catalog = dbutils.widgets.get("catalog")
schema = dbutils.widgets.get("schema")
secret_scope = dbutils.widgets.get("secret_scope")
history_months = int(dbutils.widgets.get("history_months"))

bronze_table = f"`{catalog}`.`{schema}`.bronze_steo_generation"

STEO_URL = "https://api.eia.gov/v2/steo/data/"
PAGE_LENGTH = 5000
SERIES_IDS = [
    "TOEPGEN_US",  # total
    "CLEPGEN_US",  # coal
    "NGEPGEN_US",  # natural gas
    "NUEPGEN_US",  # nuclear
    "HVEPGEN_US",  # conventional hydro
    "WNEPGEN_US",  # wind
    "SOEPGEN_US",  # utility-scale solar
    "PAEPGEN_US",  # petroleum
]

# COMMAND ----------

spark.sql(f"CREATE SCHEMA IF NOT EXISTS `{catalog}`.`{schema}`")
spark.sql(
    f"""
    CREATE TABLE IF NOT EXISTS {bronze_table} (
      vintage STRING NOT NULL,
      series_id STRING NOT NULL,
      period STRING NOT NULL,
      value STRING,
      unit STRING,
      raw_record STRING,
      ingested_at TIMESTAMP
    )
    COMMENT 'Raw EIA STEO electric power sector generation series, one row per retrieval month (vintage), series, and period.'
    """
)

# COMMAND ----------


def shift_month(period: str, months: int) -> str:
    year, month = map(int, period.split("-"))
    index = year * 12 + (month - 1) + months
    return f"{index // 12:04d}-{index % 12 + 1:02d}"


def fetch_records(api_key: str, start: str) -> list[dict]:
    records: list[dict] = []
    offset = 0
    while True:
        response = requests.get(
            STEO_URL,
            params={
                "api_key": api_key,
                "frequency": "monthly",
                "data[]": "value",
                "facets[seriesId][]": SERIES_IDS,
                "start": start,
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


vintage = date.today().strftime("%Y-%m")
start = shift_month(vintage, -history_months)
records = fetch_records(dbutils.secrets.get(secret_scope, "eia-api-key").strip(), start)
print(f"Fetched {len(records)} STEO records from {start} (vintage {vintage})")

# COMMAND ----------

ingested_at = datetime.now(timezone.utc)
rows = {
    (record["seriesId"], record["period"]): (
        vintage,
        record["seriesId"],
        record["period"],
        None if record.get("value") is None else str(record["value"]),
        record.get("unit"),
        json.dumps(record),
        ingested_at,
    )
    for record in records
}

if rows:
    spark.createDataFrame(
        list(rows.values()),
        "vintage STRING, series_id STRING, period STRING, value STRING, unit STRING, "
        "raw_record STRING, ingested_at TIMESTAMP",
    ).createOrReplaceTempView("steo_updates")

    spark.sql(
        f"""
        MERGE INTO {bronze_table} AS target
        USING steo_updates AS source
          ON target.vintage = source.vintage
         AND target.series_id = source.series_id
         AND target.period = source.period
        WHEN MATCHED THEN UPDATE SET *
        WHEN NOT MATCHED THEN INSERT *
        """
    )

print(f"Merged {len(rows)} rows into {bronze_table}")
